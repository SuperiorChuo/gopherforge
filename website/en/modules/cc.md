# Call Center

The **cc service** (call control plane) plus `freeswitch-cc/` (FreeSWITCH media + control-api) form the call center: the platform issues commands, the media plane executes, and events flow back signed with HMAC.

## Capabilities

- **Architecture**: the `cc` service drives FreeSWITCH control-api over HTTP; the media plane runs its own compose stack for independent upgrades.
- **Agents & queues**: agents, skill groups, queue strategies (ring-all …), demo queues and load scripts.
- **Blacklist & contacts**: number blacklist, contact lookup wired to CRM (`lookup has_crm`).
- **Reporting**: totals / answered / answer rate / agent leaderboards, prefix-based rate tables.
- **Media**: SIP and browser WSS calls, recordings and media files (separate media component).

## Configuration dependencies

- The control plane needs **`CC_FS_API_BASE`** pointing at control-api; without it call state / queue / report acceptance cannot be reached (fail closed, no fabricated data).
- The media plane (FreeSWITCH, SIP trunks, telephony media) is deployed separately and is not shipped in this repository.

## Boundaries

- 🧪 Experimental line: APIs and schemas may still change.
- Real telephony media / carriers / external vendor onboarding are out of scope; full acceptance requires `CC_FS_API_BASE` and a media deployment.
