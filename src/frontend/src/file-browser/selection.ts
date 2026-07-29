export type FileItemKey = string

export interface SelectionModifiers {
  readonly ctrlKey?: boolean
  readonly metaKey?: boolean
  readonly shiftKey?: boolean
}

export interface ItemSelectionCommand {
  readonly key: FileItemKey
  readonly orderedKeys: readonly FileItemKey[]
  readonly modifiers?: SelectionModifiers
}

export interface FocusMoveCommand {
  readonly orderedKeys: readonly FileItemKey[]
  readonly direction: -1 | 1 | 'first' | 'last'
  readonly modifiers?: SelectionModifiers
}

export interface FileSelectionSnapshot {
  readonly selectedKeys: readonly FileItemKey[]
  readonly anchorKey: FileItemKey | null
  readonly focusedKey: FileItemKey | null
}

export type VisibleSelectionState = 'none' | 'partial' | 'all'

export interface IFileSelectionController {
  readonly snapshot: FileSelectionSnapshot
  isSelected(key: FileItemKey): boolean
  select(command: ItemSelectionCommand): FileSelectionSnapshot
  toggle(key: FileItemKey): FileSelectionSnapshot
  toggleVisible(keys: readonly FileItemKey[]): FileSelectionSnapshot
  prepareContextMenu(key: FileItemKey): FileSelectionSnapshot
  moveFocus(command: FocusMoveCommand): FileSelectionSnapshot
  visibleState(keys: readonly FileItemKey[]): VisibleSelectionState
  replace(keys: readonly FileItemKey[]): FileSelectionSnapshot
  clear(): FileSelectionSnapshot
}

interface MutableSelectionState {
  selectedKeys: Set<FileItemKey>
  anchorKey: FileItemKey | null
  focusedKey: FileItemKey | null
}

interface SelectionStrategyContext {
  readonly state: MutableSelectionState
  readonly command: ItemSelectionCommand
}

interface ISelectionStrategy {
  apply(context: SelectionStrategyContext): MutableSelectionState
}

class ExclusiveSelectionStrategy implements ISelectionStrategy {
  apply({ command }: SelectionStrategyContext): MutableSelectionState {
    return createState([command.key], command.key, command.key)
  }
}

class ToggleSelectionStrategy implements ISelectionStrategy {
  apply({ state, command }: SelectionStrategyContext): MutableSelectionState {
    const selectedKeys = new Set(state.selectedKeys)
    if (selectedKeys.has(command.key)) selectedKeys.delete(command.key)
    else selectedKeys.add(command.key)
    return { selectedKeys, anchorKey: command.key, focusedKey: command.key }
  }
}

class RangeSelectionStrategy implements ISelectionStrategy {
  apply({ state, command }: SelectionStrategyContext): MutableSelectionState {
    const anchorIndex = state.anchorKey ? command.orderedKeys.indexOf(state.anchorKey) : -1
    const targetIndex = command.orderedKeys.indexOf(command.key)
    if (anchorIndex < 0 || targetIndex < 0) return createState([command.key], command.key, command.key)

    const start = Math.min(anchorIndex, targetIndex)
    const end = Math.max(anchorIndex, targetIndex)
    const range = command.orderedKeys.slice(start, end + 1)
    const additive = Boolean(command.modifiers?.ctrlKey || command.modifiers?.metaKey)
    return {
      selectedKeys: additive ? new Set([...state.selectedKeys, ...range]) : new Set(range),
      anchorKey: state.anchorKey,
      focusedKey: command.key,
    }
  }
}

function createState(
  selectedKeys: readonly FileItemKey[] = [],
  anchorKey: FileItemKey | null = null,
  focusedKey: FileItemKey | null = null,
): MutableSelectionState {
  return { selectedKeys: new Set(selectedKeys), anchorKey, focusedKey }
}

function toSnapshot(state: MutableSelectionState): FileSelectionSnapshot {
  return Object.freeze({
    selectedKeys: Object.freeze([...state.selectedKeys]),
    anchorKey: state.anchorKey,
    focusedKey: state.focusedKey,
  })
}

export class DesktopFileSelectionController implements IFileSelectionController {
  private state = createState()
  private readonly exclusiveStrategy: ISelectionStrategy = new ExclusiveSelectionStrategy()
  private readonly toggleStrategy: ISelectionStrategy = new ToggleSelectionStrategy()
  private readonly rangeStrategy: ISelectionStrategy = new RangeSelectionStrategy()

  get snapshot(): FileSelectionSnapshot {
    return toSnapshot(this.state)
  }

  isSelected(key: FileItemKey): boolean {
    return this.state.selectedKeys.has(key)
  }

  select(command: ItemSelectionCommand): FileSelectionSnapshot {
    const modifiers = command.modifiers
    const strategy = modifiers?.shiftKey
      ? this.rangeStrategy
      : modifiers?.ctrlKey || modifiers?.metaKey
        ? this.toggleStrategy
        : this.exclusiveStrategy
    this.state = strategy.apply({ state: this.state, command })
    return this.snapshot
  }

  toggle(key: FileItemKey): FileSelectionSnapshot {
    this.state = this.toggleStrategy.apply({
      state: this.state,
      command: { key, orderedKeys: [key] },
    })
    return this.snapshot
  }

  toggleVisible(keys: readonly FileItemKey[]): FileSelectionSnapshot {
    if (!keys.length) return this.snapshot
    const selectedKeys = new Set(this.state.selectedKeys)
    if (keys.every((key) => selectedKeys.has(key))) keys.forEach((key) => selectedKeys.delete(key))
    else keys.forEach((key) => selectedKeys.add(key))
    const focusedKey = keys[keys.length - 1] ?? this.state.focusedKey
    this.state = { selectedKeys, anchorKey: focusedKey, focusedKey }
    return this.snapshot
  }

  prepareContextMenu(key: FileItemKey): FileSelectionSnapshot {
    if (this.state.selectedKeys.has(key)) {
      this.state = { ...this.state, focusedKey: key }
      return this.snapshot
    }
    this.state = createState([key], key, key)
    return this.snapshot
  }

  moveFocus(command: FocusMoveCommand): FileSelectionSnapshot {
    if (!command.orderedKeys.length) return this.snapshot
    const currentIndex = this.state.focusedKey ? command.orderedKeys.indexOf(this.state.focusedKey) : -1
    let targetIndex: number
    if (command.direction === 'first') targetIndex = 0
    else if (command.direction === 'last') targetIndex = command.orderedKeys.length - 1
    else if (currentIndex < 0) targetIndex = command.direction > 0 ? 0 : command.orderedKeys.length - 1
    else targetIndex = Math.max(0, Math.min(command.orderedKeys.length - 1, currentIndex + command.direction))
    return this.select({
      key: command.orderedKeys[targetIndex],
      orderedKeys: command.orderedKeys,
      modifiers: command.modifiers,
    })
  }

  visibleState(keys: readonly FileItemKey[]): VisibleSelectionState {
    if (!keys.length || !keys.some((key) => this.state.selectedKeys.has(key))) return 'none'
    return keys.every((key) => this.state.selectedKeys.has(key)) ? 'all' : 'partial'
  }

  replace(keys: readonly FileItemKey[]): FileSelectionSnapshot {
    const uniqueKeys = [...new Set(keys)]
    const focusedKey = uniqueKeys[uniqueKeys.length - 1] ?? null
    this.state = createState(uniqueKeys, focusedKey, focusedKey)
    return this.snapshot
  }

  clear(): FileSelectionSnapshot {
    this.state = createState()
    return this.snapshot
  }
}
