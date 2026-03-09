// Tab definitions shared between settings panels and the sidebar navigation.
// Adding a tab here automatically adds it to the sidebar nav items.

export interface TabDef {
  id: string;
  // Dot-notation i18n key, e.g. 'settings.members'
  labelKey: string;
  // If true, the tab is hidden in the sidebar for non-admins
  adminOnly?: boolean;
}

// Workspace settings tabs in display order.
export const workspaceTabDefs: TabDef[] = [
  { id: 'members', labelKey: 'settings.members' },
  { id: 'settings', labelKey: 'settings.workspace' },
  { id: 'quotas', labelKey: 'settings.quotas' },
  { id: 'sync', labelKey: 'settings.gitSync', adminOnly: true },
];

// Organization settings tabs in display order.
export const orgTabDefs: TabDef[] = [
  { id: 'members', labelKey: 'settings.members' },
  { id: 'settings', labelKey: 'settings.settings' },
  { id: 'quotas', labelKey: 'settings.quotas' },
];
