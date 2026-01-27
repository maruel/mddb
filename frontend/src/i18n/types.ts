// Type definitions for internationalization (dictionaries and locales).

// Supported locales
export type Locale = 'en' | 'fr' | 'de' | 'es';

// Dictionary structure - English is the source of truth
export interface Dictionary {
  common: {
    loading: string;
    save: string;
    saving: string;
    saved: string;
    cancel: string;
    delete: string;
    close: string;
    back: string;
    confirm: string;
    actions: string;
    invite: string;
    push: string;
    remove: string;
    creating: string;
  };

  app: {
    title: string;
    tagline: string;
    logout: string;
    workspace: string;
    newPage: string;
    newTable: string;
    settings: string;
    privacyPolicy: string;
    terms: string;
    myOrg: string;
    createSubPage: string;
    createSubTable: string;
  };

  userMenu: {
    logout: string;
    profile: string;
  };

  profile: {
    title: string;
  };

  auth: {
    login: string;
    register: string;
    createAccount: string;
    loginTitle: string;
    email: string;
    password: string;
    name: string;
    pleaseWait: string;
    alreadyHaveAccount: string;
    dontHaveAccount: string;
    or: string;
    loginWithGoogle: string;
    loginWithMicrosoft: string;
    loginWithGitHub: string;
  };

  editor: {
    titlePlaceholder: string;
    contentPlaceholder: string;
    unsaved: string;
    history: string;
    hideHistory: string;
    versionHistory: string;
    noHistory: string;
    restoreConfirm: string;
  };

  welcome: {
    title: string;
    subtitle: string;
    createPage: string;
    createTable: string;
    welcomePageTitle: string;
    welcomePageContent: string;
  };

  onboarding: {
    welcome: string;
    letsGetStarted: string;
    confirmWorkspaceName: string;
    workspaceName: string;
    workspaceNameHint: string;
    nextGitSync: string;
    advancedSyncTitle: string;
    advancedSyncHint: string;
    repoUrl: string;
    repoUrlPlaceholder: string;
    patLabel: string;
    patPlaceholder: string;
    skipForNow: string;
    setupAndFinish: string;
    defaultOrgName: string;
    defaultOrgNameFallback: string;
    defaultWorkspaceName: string;
    defaultWorkspaceNameFallback: string;
  };

  createOrg: {
    title: string;
    description: string;
    firstOrgTitle: string;
    firstOrgDescription: string;
    nameLabel: string;
    namePlaceholder: string;
    create: string;
  };

  createWorkspace: {
    title: string;
    description: string;
    firstWorkspaceTitle: string;
    firstWorkspaceDescription: string;
    nameLabel: string;
    namePlaceholder: string;
    create: string;
  };

  settings: {
    title: string;
    members: string;
    personal: string;
    workspace: string;
    gitSync: string;
    // Members tab
    nameColumn: string;
    emailColumn: string;
    roleColumn: string;
    adminOnlyMembers: string;
    inviteNewMember: string;
    emailPlaceholder: string;
    pendingInvitations: string;
    sentColumn: string;
    // Personal tab
    personalSettings: string;
    theme: string;
    themeLight: string;
    themeDark: string;
    themeSystem: string;
    language: string;
    languageEn: string;
    languageFr: string;
    languageDe: string;
    languageEs: string;
    enableNotifications: string;
    saveChanges: string;
    // Linked accounts
    linkedAccounts: string;
    linkedAccountsHint: string;
    linkAccount: string;
    unlinkAccount: string;
    cannotUnlinkOnly: string;
    linked: string;
    notLinked: string;
    // Email change
    changeEmail: string;
    newEmail: string;
    currentPassword: string;
    emailChangeHint: string;
    emailVerified: string;
    emailNotVerified: string;
    // Workspace tab
    workspaceSettings: string;
    adminOnlyWorkspace: string;
    organizationName: string;
    workspaceName: string;
    allowPublicAccess: string;
    allowedDomains: string;
    allowedDomainsPlaceholder: string;
    allowedDomainsHint: string;
    saveWorkspaceSettings: string;
    // Git sync tab
    gitSynchronization: string;
    gitSyncHint: string;
    urlColumn: string;
    lastSyncColumn: string;
    never: string;
    addNewRemote: string;
    remoteName: string;
    remoteNamePlaceholder: string;
    repositoryUrl: string;
    repositoryUrlPlaceholder: string;
    personalAccessToken: string;
    tokenPlaceholder: string;
    tokenHint: string;
    addRemote: string;
    confirmRemoveRemote: string;
    // Roles
    roleAdmin: string;
    roleEditor: string;
    roleViewer: string;
  };

  table: {
    records: string;
    table: string;
    grid: string;
    gallery: string;
    board: string;
    noRecords: string;
    loadMore: string;
    deleteRecord: string;
    confirmDeleteRecord: string;
    untitled: string;
    noImage: string;
    noGroup: string;
    addSelectColumn: string;
  };

  errors: {
    VALIDATION_FAILED: string;
    MISSING_FIELD: string;
    INVALID_FORMAT: string;
    NOT_FOUND: string;
    NODE_NOT_FOUND: string;
    TABLE_NOT_FOUND: string;
    FILE_NOT_FOUND: string;
    STORAGE_ERROR: string;
    INTERNAL_ERROR: string;
    NOT_IMPLEMENTED: string;
    CONFLICT: string;
    UNAUTHORIZED: string;
    FORBIDDEN: string;
    unknown: string;
    sessionExpired: string;
    titleRequired: string;
    failedToLoad: string;
    failedToSave: string;
    failedToDelete: string;
    failedToCreate: string;
    failedToSwitch: string;
    failedToInvite: string;
    failedToUpdateRole: string;
    failedToAddRemote: string;
    failedToRemoveRemote: string;
    pushFailed: string;
    autoSaveFailed: string;
    noAccessToOrg: string;
    noAccessToWs: string;
  };

  success: {
    invitationSent: string;
    roleUpdated: string;
    personalSettingsSaved: string;
    workspaceSettingsSaved: string;
    remoteAdded: string;
    remoteRemoved: string;
    pushSuccessful: string;
  };

  pwa: {
    installTitle: string;
    installMessage: string;
    installButton: string;
    dismissButton: string;
  };
}
