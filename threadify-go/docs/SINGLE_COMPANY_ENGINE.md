# One company per Engine

An Engine installation and its PostgreSQL database serve one company. The
Registry binding fixes the company identity, and the `companies` table rejects
a second row. Use a separate Engine and database for another company.

The database still stores `company_id` on records so requests and archived
events can be checked against the bound company. Its indexes use the fields
needed for lookups and ordering without repeating that constant company ID.
Uniqueness for contracts, profiles, billing cycles, and managed identities is
also installation-wide.

Schema initialization replaces older company-prefixed indexes and unique
constraints. An existing database with more than one company fails the
single-company check without deleting or merging records; move those companies
to separate installations before upgrading.
