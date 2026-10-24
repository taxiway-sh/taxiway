"""Preserve signed, omitted thinking; source compatibility checked on LiteLLM 1.106.3.

Opus/Sonnet 5.5 and Fable return empty thinking text with an opaque signature.
Anthropic requires replaying those blocks unchanged across tool turns:
https://platform.claude.com/docs/en/models/opus-5-5/migration-guide
Upstream now preserves these blocks with is_empty_unsigned_thinking_block.
This compatible wrapper stays until the shipped image's protocol test proves
the same behavior without it; the source check alone is not runtime validation.
Remove this shim once the shipped image passes the protocol test without it.
"""

from litellm.integrations.custom_logger import CustomLogger
from litellm.llms.anthropic import common_utils


_original_is_empty = getattr(common_utils, "is_empty_thinking_block", None)
if not callable(_original_is_empty):
    raise RuntimeError("Anthropic shim incompatible with this LiteLLM version; see gateway protocol test")


def _is_empty_unsigned(block):
    if (isinstance(block, dict) and block.get("type") == "thinking"
            and isinstance(block.get("signature"), str) and block["signature"]):
        return False
    return _original_is_empty(block)


common_utils.is_empty_thinking_block = _is_empty_unsigned
proxy_handler_instance = CustomLogger()
