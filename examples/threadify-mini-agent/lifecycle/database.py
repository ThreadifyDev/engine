from harnest import lifecycle
from harnest.lib.support_db import initialize


@lifecycle.resource
def support_database():
    initialize()
    yield
