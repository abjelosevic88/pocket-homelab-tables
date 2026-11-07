import { Modal } from './ui'

const GROUPS: { title: string; keys: [string[], string][] }[] = [
  { title: 'Go to', keys: [[['h'], 'Home'], [[','], 'Settings'], [['/'], 'Search & commands'], [['⌘', 'K'], 'Search & commands']] },
  { title: 'Make', keys: [[['n'], 'A new base'], [['r'], 'A new row (in a table)']] },
  { title: 'In the grid', keys: [[['↑', '↓', '←', '→'], 'Move between cells'], [['Enter'], 'Change the cell'], [['⇧', 'Enter'], 'Open the row'], [['Space'], 'Open the row, or tick a box'], [['Delete'], 'Clear the cell'], [['⌘', 'C'], 'Copy the cell'], [['⌘', 'V'], 'Paste (many cells from a spreadsheet)'], [['Esc'], 'Stop']] },
  { title: 'A row', keys: [[['alt', '←'], 'The row before'], [['alt', '→'], 'The row after'], [['⌘', 'Enter'], 'Send a comment']] },
  { title: 'Window', keys: [[['['], 'Show or hide the sidebar'], [['esc'], 'Close a dialog or menu']] },
  { title: 'Help', keys: [[['?'], 'This sheet']] },
]

export function ShortcutsSheet({ onClose }: { onClose: () => void }) {
  return (
    <Modal title="Keyboard shortcuts" onClose={onClose} size="lg">
      <div className="shortcuts">
        {GROUPS.map(g => (
          <section key={g.title}>
            <h4 className="sc-h">{g.title}</h4>
            {g.keys.map(([keys, label]) => (
              <div key={label + keys.join('')} className="sc-row"><span>{label}</span><span className="sc-keys">{keys.map(k => <kbd key={k}>{k}</kbd>)}</span></div>
            ))}
          </section>
        ))}
      </div>
    </Modal>
  )
}
