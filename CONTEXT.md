# Domain context

## File browser

- **Current Directory** — The directory whose entries are visible in a File Browser. Browser Selection is scoped to it and is cleared when it changes.
- **Browser Selection** — The visible entries affected by the next file action. A plain item click replaces it; Ctrl/Cmd toggles an item; Shift selects a range; a checkbox toggles without clearing other items.
- **Selection Anchor** — The entry used as the fixed end of a Shift range selection.
- **Transfer Source** — A `SourceLocator` captured from Browser Selection when the user starts an fnOS-to-PS5 transfer. It belongs to the transfer workflow, not the Library browser state.
- **Directory Target** — The folder chosen for an upload or download. It can differ from the Current Directory while the user browses the picker.

## Interaction invariants

- Opening a folder or file never adds or removes a Browser Selection item.
- Right-clicking an already selected entry preserves Browser Selection; right-clicking an unselected entry replaces it.
- Checkbox selection and item selection operate on the same Browser Selection through different commands.
- Pickers never expose mutation actions or Browser Selection checkboxes.
