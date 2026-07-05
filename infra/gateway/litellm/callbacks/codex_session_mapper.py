import json

from litellm.integrations.custom_logger import CustomLogger


class TaxiwayCodexSessionMapper(CustomLogger):
    async def async_pre_call_hook(self, user_api_key_dict, cache, data, call_type):
        self._apply_codex_session(data)
        return data

    async def async_logging_hook(self, kwargs, result, call_type):
        self._apply_codex_session(kwargs)
        return kwargs, result

    async def async_log_success_event(self, kwargs, response_obj, start_time, end_time):
        self._apply_codex_session(kwargs)

    async def async_log_stream_event(self, kwargs, response_obj, start_time, end_time):
        self._apply_codex_session(kwargs)

    def _apply_codex_session(self, data):
        litellm_params = data.setdefault("litellm_params", {})
        metadata = litellm_params.setdefault("metadata", {})
        request_metadata = data.setdefault("metadata", {})
        headers = self._request_headers(litellm_params)

        session_id = self._codex_session_id(headers)
        if session_id:
            litellm_params["litellm_session_id"] = session_id
            metadata["session_id"] = session_id
            request_metadata["session_id"] = session_id
            standard_logging_object = data.get("standard_logging_object")
            if isinstance(standard_logging_object, dict):
                standard_logging_object["trace_id"] = session_id

    def _codex_session_id(self, headers):
        turn_metadata = headers.get("x-codex-turn-metadata")
        if turn_metadata:
            try:
                parsed = json.loads(turn_metadata)
            except (TypeError, ValueError):
                parsed = {}

            session_id = parsed.get("session_id") or parsed.get("thread_id")
            if isinstance(session_id, str) and session_id:
                return session_id

        return None

    def _request_headers(self, litellm_params):
        request = litellm_params.get("proxy_server_request", {}) or {}
        headers = request.get("headers", {}) or {}
        return {str(key).lower(): value for key, value in headers.items()}


proxy_handler_instance = TaxiwayCodexSessionMapper()
