// The other Pocket apps a table can reach, and words for a connection's state.
import type { AppSlug, AppStateName } from './types'

export const APP_INFO: Record<AppSlug, { name: string; icon: string; what: string }> = {
  contacts: { name: 'Pocket Contacts', icon: 'users', what: 'people' },
  tasks: { name: 'Pocket Tasks', icon: 'check-square', what: 'tasks' },
  calendar: { name: 'Pocket Calendar', icon: 'calendar', what: 'events' },
  notes: { name: 'Pocket Notes', icon: 'note', what: 'notes' },
  forms: { name: 'Pocket Forms', icon: 'list', what: 'answers to forms' },
  files: { name: 'Pocket Files', icon: 'file', what: 'spreadsheet files' },
  database: { name: 'Pocket Database', icon: 'database', what: 'your databases' },
  documents: { name: 'Pocket Documents', icon: 'archive', what: 'documents' },
  inventory: { name: 'Pocket Inventory', icon: 'package', what: 'things you own' },
  places: { name: 'Pocket Places', icon: 'pin', what: 'places' },
  crm: { name: 'Pocket CRM', icon: 'trophy', what: 'deals' },
  bookshelf: { name: 'Pocket Bookshelf', icon: 'journal', what: 'books' },
  wishlist: { name: 'Pocket Wishlist', icon: 'gift', what: 'wishes' },
  money: { name: 'Pocket Money', icon: 'wallet', what: 'payments' },
}

/** Short words for a state that does not work, for a greyed-out button. */
export const STATE_WORDS: Record<AppStateName, string> = {
  on: 'Connected', partly: 'Partly connected', off: 'Switched off here', link_off: 'Switched off in Pocket Core', missing: 'Not on this box',
  old: 'Needs a newer version', no_core: 'No Pocket Core', not_set_up: 'Not set up', error: 'Not reachable',
}
