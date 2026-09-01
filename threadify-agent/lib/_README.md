# Reusable Python helpers

Add ordinary Python modules here when agent resources need the same
implementation. Import a helper through the compiler-owned namespace:

    from harnest.lib.audit import record_change

Nested helper modules follow the same import path. The root-only lib/ directory
is bundled but never discovered as tools or other agent resources. Keep resource
declarations in their owning folders. Harnest ignores this underscore-prefixed
guide; replace it with Python modules as needed.
