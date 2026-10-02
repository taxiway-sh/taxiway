"""Run inside the shipped LiteLLM image, with no external network or credentials."""

import json
import os
import queue
import subprocess
import tempfile
import threading
import time
import urllib.error
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import yaml


received = queue.Queue()
responses = queue.Queue()
provider_errors = {}
auth_mode = os.environ.get("TEST_AUTH_MODE", "subscription")


class Anthropic(BaseHTTPRequestHandler):
    def log_message(self, *args):
        pass

    def do_POST(self):
        payload = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        received.put((self.path, dict(self.headers), payload))
        failure = provider_errors.get(payload.get("model"))
        status, content_type, body = failure if failure else responses.get(timeout=10)
        self.send_response(status)
        self.send_header("Content-Type", content_type)
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)


def message(model, content, reason="end_turn"):
    return dict(id="msg_test", type="message", role="assistant", model=model,
                content=content, stop_reason=reason, stop_sequence=None,
                usage=dict(input_tokens=10, output_tokens=4))


def request(payload, endpoint="/v1/messages"):

    headers = {"Content-Type": "application/json",
               "x-litellm-api-key": "Bearer sk-test-only-protocol",
               "anthropic-version": "2023-06-01"}
    if auth_mode == "subscription":
        headers["Authorization"] = "Bearer sk-ant-oat01-test-only-not-a-real-token"
    req = urllib.request.Request(
        "http://127.0.0.1:4000" + endpoint, data=json.dumps(payload).encode(),
        headers=headers)
    with urllib.request.urlopen(req, timeout=20) as response:
        return response.read()


def run_error_checks(models, endpoint):
    for model, (status, error_type) in zip(models, [(401, "authentication_error"), (429, "rate_limit_error")]):
        body = dict(type="error", error=dict(type=error_type, message="Fixture provider error"))
        provider_errors[model] = (status, "application/json", json.dumps(body).encode())
        payload = dict(model=model, max_tokens=128, messages=[dict(role="user", content="Hello")])
        if endpoint == "/v1/responses":
            payload = dict(model=model, stream=True, input="Hello")
        try:
            request(payload, endpoint)
            raise AssertionError("Provider failure was hidden")
        except urllib.error.HTTPError as error:
            assert error.code == status, (status, error.code, error.read().decode())
        _, _, upstream = received.get(timeout=2)
        assert upstream["model"] == model, upstream
        while not received.empty():
            _, _, retry = received.get_nowait()
            assert retry["model"] == model, retry
    print("PASS provider authentication and rate-limit errors", flush=True)


def run_checks(models):
    for model in models:
        payload = dict(model=model, max_tokens=128,
                       messages=[dict(role="user", content="Hello")])
        expected = message(model, [dict(type="text", text="Hello from fixture")])
        responses.put((200, "application/json", json.dumps(expected).encode()))
        result = json.loads(request(payload))
        assert result["content"] == expected["content"], result
        assert result["stop_reason"] == "end_turn", result
        path, headers, upstream = received.get(timeout=2)
        assert path.split("?")[0] == "/v1/messages", path
        assert upstream["model"] == model, upstream
        assert upstream["messages"] == payload["messages"], upstream
        headers = {k.lower(): v for k, v in headers.items()}
        assert "x-litellm-api-key" not in headers, "gateway key must not reach provider"
        if auth_mode == "subscription":
            assert headers["authorization"] == "Bearer sk-ant-oat01-test-only-not-a-real-token"
        else:
            assert headers["x-api-key"] == "test-provider-key"
        print("PASS routing and credential forwarding:", model, flush=True)

    model = "claude-opus-5-5"
    thinking = dict(type="thinking", thinking="", signature="test-opaque-signature")
    tool = dict(type="tool_use", id="toolu_test", name="read_file", input={"path": "README.md"})
    tools = [dict(name="read_file", description="Read a file", input_schema={
        "type": "object", "properties": {"path": {"type": "string"}}, "required": ["path"]})]
    payload = dict(model=model, max_tokens=1024, thinking={"type": "adaptive"},
                   tools=tools, messages=[dict(role="user", content="Read README.md")])
    expected = message(model, [thinking, tool], "tool_use")
    responses.put((200, "application/json", json.dumps(expected).encode()))
    result = json.loads(request(payload))
    assert result["content"] == [thinking, tool], result
    assert result["stop_reason"] == "tool_use", result
    received.get(timeout=2)
    payload["messages"] += [dict(role="assistant", content=result["content"]),
                            dict(role="user", content=[dict(type="tool_result", tool_use_id="toolu_test", content="File contents")])]
    responses.put((200, "application/json", json.dumps(message(model, [dict(type="text", text="Done")])).encode()))
    request(payload)
    _, _, upstream = received.get(timeout=2)
    assert upstream["messages"] == payload["messages"], "LiteLLM removed signed thinking from tool history"
    assert upstream["thinking"] == {"type": "adaptive"}, upstream
    assert upstream["tools"] == tools, upstream
    print("PASS tool round trip and opaque thinking signature", flush=True)

    events = [dict(type="message_start", message=message(model, [], None)),
              dict(type="content_block_start", index=0, content_block=dict(type="thinking", thinking="", signature="")),
              dict(type="content_block_delta", index=0, delta=dict(type="signature_delta", signature="test-opaque-signature")),
              dict(type="content_block_stop", index=0),
              dict(type="content_block_start", index=1, content_block=dict(type="text", text="")),
              dict(type="content_block_delta", index=1, delta=dict(type="text_delta", text="Hello")),
              dict(type="content_block_stop", index=1),
              dict(type="message_delta", delta=dict(stop_reason="end_turn", stop_sequence=None), usage=dict(output_tokens=4)),
              dict(type="message_stop")]
    stream = "".join("event: " + e["type"] + "\ndata: " + json.dumps(e) + "\n\n" for e in events)
    responses.put((200, "text/event-stream", stream.encode()))
    payload["stream"] = True
    raw = request(payload).decode()
    actual = [json.loads(line[6:]) for line in raw.splitlines() if line.startswith("data: ")]
    assert [e["type"] for e in actual] == [e["type"] for e in events], actual
    assert actual[2]["delta"] == events[2]["delta"], actual
    assert actual[5]["delta"] == events[5]["delta"], actual
    assert actual[-2]["delta"]["stop_reason"] == "end_turn", actual
    received.get(timeout=2)
    print("PASS streaming text and thinking signature", flush=True)
    run_error_checks(models[-2:], "/v1/messages")



def codex_response(model, output):
    return dict(id="resp_fixture", object="response", created_at=1790950000,
                status="completed", model=model, output=output,
                error=None, incomplete_details=None,
                usage=dict(input_tokens=10, output_tokens=4, total_tokens=14,
                           input_tokens_details=dict(cached_tokens=0),
                           output_tokens_details=dict(reasoning_tokens=0)))


def codex_events(model, output):
    response = codex_response(model, output)
    events = [dict(type="response.created", response={**response, "status": "in_progress", "output": []}, sequence_number=0)]
    for index, item in enumerate(output):
        events.append(dict(type="response.output_item.done", output_index=index, item=item, sequence_number=len(events)))
    events.append(dict(type="response.completed", response=response, sequence_number=len(events)))
    return "".join("event: " + e["type"] + "\ndata: " + json.dumps(e) + "\n\n" for e in events).encode()


def run_codex_checks(models):
    text = dict(type="message", id="msg_fixture", role="assistant", status="completed",
                content=[dict(type="output_text", text="Hello", annotations=[])])
    for model in models:
        payload = dict(model=model, stream=True, input=[dict(role="user", content="Hello")],
                       reasoning=dict(effort="medium"), parallel_tool_calls=False)
        responses.put((200, "text/event-stream", codex_events(model, [text])))
        raw = request(payload, "/v1/responses").decode()
        events = [json.loads(line[6:]) for line in raw.splitlines() if line.startswith("data: ") and line[6:] != "[DONE]"]
        assert events[-1]["type"] == "response.completed", events
        result = events[-1]["response"]
        assert result["output"][0]["content"][0]["text"] == "Hello", result
        assert result["model"] == model, result
        path, headers, upstream = received.get(timeout=2)
        assert path == "/backend-api/codex/responses", path
        assert upstream["model"] == model, upstream
        assert upstream["input"] == payload["input"], upstream
        assert upstream["reasoning"] == payload["reasoning"], upstream
        assert upstream.get("parallel_tool_calls") is False, "Codex Responses Lite requires parallel_tool_calls=false"
        assert upstream["stream"] is True and upstream["store"] is False, upstream
        assert "reasoning.encrypted_content" in upstream["include"], upstream
        headers = {k.lower(): v for k, v in headers.items()}
        assert headers["authorization"] == "Bearer test-only-codex-token", headers
        assert headers["chatgpt-account-id"] == "test-only-account", headers
        assert "x-litellm-api-key" not in headers, "gateway key reached provider"
        print("PASS Codex Responses routing:", model, flush=True)

    model = models[0]
    reasoning = dict(type="reasoning", id="rs_fixture", summary=[], encrypted_content="opaque-test-reasoning")
    tool = dict(type="function_call", id="fc_fixture", call_id="call_fixture", name="read_file",
                arguments='{"path":"README.md"}', status="completed")
    payload = dict(model=model, stream=True, input=[dict(role="user", content="Read README")],
                   tools=[dict(type="function", name="read_file", parameters=dict(type="object", properties={}))],
                   parallel_tool_calls=True)
    responses.put((200, "text/event-stream", codex_events(model, [reasoning, tool])))
    raw = request(payload, "/v1/responses").decode()
    events = [json.loads(line[6:]) for line in raw.splitlines() if line.startswith("data: ") and line[6:] != "[DONE]"]
    assert events[-1]["type"] == "response.completed", events
    output = events[-1]["response"]["output"]
    assert output[0]["encrypted_content"] == reasoning["encrypted_content"], output
    assert output[1]["call_id"] == tool["call_id"], output
    received.get(timeout=2)
    payload["input"] += output + [dict(type="function_call_output", call_id="call_fixture", output="File contents")]
    payload["stream"] = True
    responses.put((200, "text/event-stream", codex_events(model, [text])))
    request(payload, "/v1/responses")
    _, _, upstream = received.get(timeout=2)
    assert upstream["input"] == payload["input"], upstream
    assert upstream["tools"] == payload["tools"], upstream
    assert upstream.get("parallel_tool_calls") is True, "Explicit parallel tool calls must also be preserved"
    print("PASS Codex streaming, encrypted reasoning and tool replay", flush=True)
    run_error_checks(models[-2:], "/v1/responses")


server = ThreadingHTTPServer(("127.0.0.1", 0), Anthropic)
threading.Thread(target=server.serve_forever, daemon=True).start()
with open("/test/config.yaml") as source:
    config = yaml.safe_load(source)
# These checks isolate the provider protocol; persistence and telemetry are tested separately.
config["general_settings"].pop("database_url", None)
config["litellm_settings"]["callbacks"] = [
    callback for callback in config["litellm_settings"]["callbacks"] if callback != "langfuse_otel"
]
models = []
for entry in config["model_list"]:
    provider = "chatgpt/" if auth_mode == "chatgpt" else "anthropic/"
    assert entry["litellm_params"]["model"].startswith(provider), entry
    models.append(entry["model_name"])
    entry["litellm_params"]["api_base"] = "http://127.0.0.1:" + str(server.server_port)
    if auth_mode == "chatgpt":
        entry["litellm_params"]["api_base"] += "/backend-api/codex"
    if auth_mode == "api-key":
        entry["litellm_params"]["api_key"] = "test-provider-key"
if auth_mode == "chatgpt":
    token_dir = tempfile.mkdtemp(prefix="test-only-codex-")
    os.environ["CHATGPT_TOKEN_DIR"] = token_dir
    os.environ["CHATGPT_API_BASE"] = "http://127.0.0.1:" + str(server.server_port) + "/backend-api/codex"
    with open(os.path.join(token_dir, "auth.json"), "w") as output:
        json.dump(dict(access_token="test-only-codex-token", account_id="test-only-account", expires_at=32503680000), output)
with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as runtime_config:
    yaml.safe_dump(config, runtime_config)
    runtime_config.flush()
    logs = tempfile.TemporaryFile(mode="w+")
    process = subprocess.Popen(["litellm", "--config", runtime_config.name, "--port", "4000"], stdout=logs, stderr=logs)
    try:
        for attempt in range(60):
            try:
                with urllib.request.urlopen("http://127.0.0.1:4000/health/liveliness", timeout=1):
                    break
            except (OSError, TimeoutError):
                if process.poll() is not None:
                    raise RuntimeError("LiteLLM exited before readiness")
                time.sleep(1)
        else:
            raise RuntimeError("LiteLLM readiness timeout")
        if auth_mode == "chatgpt":
            run_codex_checks(models)
        else:
            run_checks(models)
    except Exception:
        logs.seek(0)
        print(logs.read()[-2400:], flush=True)
        raise
    finally:
        process.terminate()
        try:
            process.wait(timeout=10)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait()
        server.shutdown()
