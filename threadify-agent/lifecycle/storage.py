from harnest.lib.storage import store
from harnest.lifecycle import lifecycle


@lifecycle.session_store
def session_store():
    """Return the shared store for completed conversations and business state."""

    return store


@lifecycle.checkpointer
def checkpointer():
    """Return the same store for private in-progress execution state."""

    return store
