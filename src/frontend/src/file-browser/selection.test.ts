import { describe, expect, it } from 'vitest'
import { DesktopFileSelectionController } from './selection'

const keys = ['a', 'b', 'c', 'd']

describe('DesktopFileSelectionController', () => {
  it('uses a plain click as an exclusive selection', () => {
    const selection = new DesktopFileSelectionController()
    selection.toggle('a')
    selection.select({ key: 'b', orderedKeys: keys })
    expect(selection.snapshot.selectedKeys).toEqual(['b'])
  })

  it('uses Ctrl or Command to toggle without clearing other items', () => {
    const selection = new DesktopFileSelectionController()
    selection.select({ key: 'a', orderedKeys: keys })
    selection.select({ key: 'c', orderedKeys: keys, modifiers: { ctrlKey: true } })
    expect(selection.snapshot.selectedKeys).toEqual(['a', 'c'])
    selection.select({ key: 'a', orderedKeys: keys, modifiers: { metaKey: true } })
    expect(selection.snapshot.selectedKeys).toEqual(['c'])
  })

  it('uses Shift to select a contiguous range from the anchor', () => {
    const selection = new DesktopFileSelectionController()
    selection.select({ key: 'b', orderedKeys: keys })
    selection.select({ key: 'd', orderedKeys: keys, modifiers: { shiftKey: true } })
    expect(selection.snapshot.selectedKeys).toEqual(['b', 'c', 'd'])
  })

  it('uses Ctrl+Shift to add a range to the existing selection', () => {
    const selection = new DesktopFileSelectionController()
    selection.select({ key: 'a', orderedKeys: keys })
    selection.select({ key: 'c', orderedKeys: keys, modifiers: { ctrlKey: true } })
    selection.select({ key: 'd', orderedKeys: keys, modifiers: { ctrlKey: true, shiftKey: true } })
    expect(selection.snapshot.selectedKeys).toEqual(['a', 'c', 'd'])
  })

  it('toggles a checkbox without clearing the other selection', () => {
    const selection = new DesktopFileSelectionController()
    selection.select({ key: 'a', orderedKeys: keys })
    selection.toggle('c')
    expect(selection.snapshot.selectedKeys).toEqual(['a', 'c'])
  })

  it('preserves a multi-selection when opening the context menu on a selected item', () => {
    const selection = new DesktopFileSelectionController()
    selection.replace(['a', 'b'])
    selection.prepareContextMenu('b')
    expect(selection.snapshot.selectedKeys).toEqual(['a', 'b'])
  })

  it('replaces selection when opening the context menu on an unselected item', () => {
    const selection = new DesktopFileSelectionController()
    selection.replace(['a', 'b'])
    selection.prepareContextMenu('c')
    expect(selection.snapshot.selectedKeys).toEqual(['c'])
  })

  it('selects and clears only visible entries from the header checkbox', () => {
    const selection = new DesktopFileSelectionController()
    selection.replace(['outside', 'a'])
    expect(selection.visibleState(keys)).toBe('partial')
    selection.toggleVisible(keys)
    expect(selection.snapshot.selectedKeys).toEqual(['outside', 'a', 'b', 'c', 'd'])
    expect(selection.visibleState(keys)).toBe('all')
    selection.toggleVisible(keys)
    expect(selection.snapshot.selectedKeys).toEqual(['outside'])
  })

  it('moves focus through the ordered entries and updates selection', () => {
    const selection = new DesktopFileSelectionController()
    selection.moveFocus({ orderedKeys: keys, direction: 1 })
    selection.moveFocus({ orderedKeys: keys, direction: 1 })
    expect(selection.snapshot.focusedKey).toBe('b')
    expect(selection.snapshot.selectedKeys).toEqual(['b'])
  })
})
