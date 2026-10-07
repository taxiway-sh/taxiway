"""Price ChatGPT subscription routes as their OpenAI API models in LiteLLM 1.103.2.

LiteLLM prices chatgpt/<model> calls from cost map entries without per-token
prices, and ignores model_info.base_model for that provider, so every call is
recorded with cost 0. Register the OpenAI API prices from the cost map loaded at
startup for those entries: the cost estimates API usage, not the subscription.
Registered prices survive cost map reloads and reset LiteLLM's lookup caches.
Remove this shim once the shipped image passes assert:generation-cost-provided
for ChatGPT routes without it.
"""

import litellm
from litellm.integrations.custom_logger import CustomLogger


_model_cost = getattr(litellm, "model_cost", None)
if not isinstance(_model_cost, dict) or not callable(getattr(litellm, "register_model", None)):
    raise RuntimeError("ChatGPT pricing shim incompatible with this LiteLLM version; "
                       "see the assert:generation-cost-provided cost test")

_prices = {}
for _key, _entry in _model_cost.items():
    if not _key.startswith("chatgpt/") or not isinstance(_entry, dict):
        continue
    _api = _model_cost.get(_key.removeprefix("chatgpt/"))
    if not isinstance(_api, dict) or _api.get("litellm_provider") != "openai":
        continue
    _missing = {field: value for field, value in _api.items()
                if "cost" in field and _entry.get(field) is None}
    if _missing:
        _prices[_key] = {**_entry, **_missing}
if _prices:
    litellm.register_model(_prices)

proxy_handler_instance = CustomLogger()
