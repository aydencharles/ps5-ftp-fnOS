# Domain context

## File browser

- **PS5 Profile** — A persisted connection configuration for one PS5 FTP endpoint, including its display name, host, credentials, and allowed base path.
- **Selected PS5 Profile** — The PS5 Profile currently used by remote browsing and transfer commands. Its ID is application state, not a durable reference after the Profile is deleted.
- **Current Directory** — The directory whose entries are visible in a File Browser. Browser Selection is scoped to it and is cleared when it changes.
- **Browser Selection** — The visible entries affected by the next file action. A plain item click replaces it; Ctrl/Cmd toggles an item; Shift selects a range; a checkbox toggles without clearing other items.
- **Selection Anchor** — The entry used as the fixed end of a Shift range selection.
- **Transfer Source** — A `SourceLocator` captured from Browser Selection when the user starts an fnOS-to-PS5 transfer. It belongs to the transfer workflow, not the Library browser state.
- **Directory Target** — The folder chosen for an upload or download. It can differ from the Current Directory while the user browses the picker.

## Interaction invariants

- Selected PS5 Profile is either empty or references an existing PS5 Profile. Refreshing or deleting Profiles must select a remaining Profile or clear the selection before another remote request can start.
- Opening a folder or file never adds or removes a Browser Selection item.
- Right-clicking an already selected entry preserves Browser Selection; right-clicking an unselected entry replaces it.
- Checkbox selection and item selection operate on the same Browser Selection through different commands.
- Pickers never expose mutation actions or Browser Selection checkboxes.
