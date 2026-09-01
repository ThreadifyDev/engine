# Pydantic contracts

Store request, response, tool, WebSocket, and streaming Pydantic models here.
Import them through the compiler-owned namespace:

    from harnest.models.support import SupportRequest, SupportResponse

Nested modules use the same harnest.models.* path. This root-only folder is
bundled but never discovered as a capability. Harnest ignores this
underscore-prefixed guide; replace it with Python modules as needed.
