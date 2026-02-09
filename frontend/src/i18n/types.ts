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
    collapseSidebar: string;
    otherWorkspaces: string;
    switchWorkspace: string;
  };

  userMenu: {
    logout: string;
    profile: string;
  };

  profile: {
    title: string;
    myOrganizations: string;
    noOrganizations: string;
    adminSettings: string;
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
    wysiwygMode: string;
    markdownMode: string;
    versionPreview: string;
    closePreview: string;
    deleteBlock: string;
    deleteBlocks: string;
    duplicateBlock: string;
    duplicateBlocks: string;
    indent: string;
    outdent: string;
    convertTo: string;
    paragraph: string;
    heading: string;
    bulletList: string;
    numberedList: string;
    taskList: string;
    blockquote: string;
    codeBlock: string;
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
    workspaces: string;
    organizations: string;
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
    // Password management
    addPassword: string;
    changePassword: string;
    addPasswordHint: string;
    changePasswordHint: string;
    newPassword: string;
    confirmPassword: string;
    passwordMismatch: string;
    passwordChanged: string;
    passwordAdded: string;
    // Linking feedback
    accountLinked: string;
    accountUnlinked: string;
    linkingFailed: string;
    // Workspace tab
    workspaceSettings: string;
    adminOnlyWorkspace: string;
    organizationName: string;
    workspaceName: string;
    clickToEditName: string;
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
    // GitHub App sync
    connectGitHub: string;
    selectRepository: string;
    gitHubAppSetup: string;
    manualSetup: string;
    autoPush: string;
    autoPushHint: string;
    pull: string;
    syncStatusIdle: string;
    syncStatusSyncing: string;
    syncStatusError: string;
    syncStatusConflict: string;
    conflictMessage: string;
    branchColumn: string;
    statusColumn: string;
    // Roles
    roleAdmin: string;
    roleEditor: string;
    roleViewer: string;
    roleOwner: string;
    roleMember: string;
    // Organization settings
    organizationSettings: string;
    organizationMembers: string;
    organizationPreferences: string;
    organizationQuotas: string;
    maxWorkspacesPerOrg: string;
    maxMembersPerOrg: string;
    maxMembersPerWorkspace: string;
    maxTotalStorageBytes: string;
    maxTablesPerWorkspace: string;
    maxColumnsPerTable: string;
    // Workspace quotas
    workspaceQuotas: string;
    maxPages: string;
    maxStorageBytes: string;
    maxRecordsPerTable: string;
    maxAssetSizeBytes: string;
    orgSettingsHint: string;
    openOrgSettings: string;
    adminOnlySettings: string;
    settings: string;
    actionsColumn: string;
    confirmRemoveMember: string;
  };

  table: {
    records: string;
    table: string;
    grid: string;
    list: string;
    gallery: string;
    board: string;
    noRecords: string;
    noColumns: string;
    loadMore: string;
    deleteRecord: string;
    confirmDeleteRecord: string;
    confirmDeleteView: string;
    untitled: string;
    noImage: string;
    noGroup: string;
    addSelectColumn: string;
    newView: string;
    defaultView: string;
    all: string;
    addColumn: string;
    addRecord: string;
    addColumnFirst: string;
    columnName: string;
    columnType: string;
    typeText: string;
    typeNumber: string;
    typeCheckbox: string;
    typeDate: string;
    typeSelect: string;
    typeUrl: string;
    typeEmail: string;
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
    failedToMove: string;
    pushFailed: string;
    pullFailed: string;
    autoSaveFailed: string;
    noAccessToOrg: string;
    noAccessToWs: string;
    failedToRemoveMember: string;
  };

  success: {
    invitationSent: string;
    roleUpdated: string;
    personalSettingsSaved: string;
    workspaceSettingsSaved: string;
    orgSettingsSaved: string;
    remoteAdded: string;
    remoteRemoved: string;
    pushSuccessful: string;
    pullSuccessful: string;
    gitHubAppConfigured: string;
    memberRemoved: string;
  };

  pwa: {
    installTitle: string;
    installMessage: string;
    installButton: string;
    dismissButton: string;
  };

  notionImport: {
    title: string;
    description: string;
    notionToken: string;
    notionTokenPlaceholder: string;
    setupTitle: string;
    step1Header: string;
    step1a: string;
    step1b: string;
    step1c: string;
    step1d: string;
    step2Header: string;
    step2a: string;
    step2b: string;
    step3Header: string;
    step3a: string;
    step3b: string;
    createIntegration: string;
    startImport: string;
    importing: string;
    cancel: string;
    // Status
    statusRunning: string;
    statusCompleted: string;
    statusFailed: string;
    statusCancelled: string;
    // Progress banner
    importInProgress: string;
    importCompleted: string;
    importFailed: string;
    importCancelled: string;
    // Stats
    pagesImported: string;
    databasesImported: string;
    recordsImported: string;
    assetsImported: string;
    errorsEncountered: string;
    dismiss: string;
  };
  slashMenu: {
    noResults: string;
    paragraph: string;
    heading1: string;
    heading2: string;
    heading3: string;
    bulletList: string;
    orderedList: string;
    taskList: string;
    blockquote: string;
    codeBlock: string;
    divider: string;
    subpage: string;
    untitledSubpage: string;
  };

  server: {
    title: string;
    serverSettings: string;
    smtpConfiguration: string;
    smtpHost: string;
    smtpPort: string;
    smtpUsername: string;
    smtpPassword: string;
    smtpPasswordHint: string;
    smtpFrom: string;
    quotas: string;
    maxRequestBodyBytes: string;
    maxSessionsPerUser: string;
    maxTablesPerWorkspace: string;
    maxColumnsPerTable: string;
    maxRecordsPerTable: string;
    maxPages: string;
    maxStorageBytes: string;
    maxOrganizations: string;
    maxWorkspaces: string;
    maxUsers: string;
    maxTotalStorageBytes: string;
    maxAssetSizeBytes: string;
    maxEgressBandwidthBps: string;
    rateLimits: string;
    rateLimitsHint: string;
    authRatePerMin: string;
    writeRatePerMin: string;
    readAuthRatePerMin: string;
    readUnauthRatePerMin: string;
    saveConfiguration: string;
    configurationSaved: string;
    smtpEnabled: string;
    smtpDisabled: string;
    // Dashboard tab
    dashboard: string;
    totalUsers: string;
    totalOrganizations: string;
    totalWorkspaces: string;
    totalStorage: string;
    activeSessions: string;
    organizationName: string;
    workspaceName: string;
    members: string;
    pages: string;
    storage: string;
    created: string;
    gitCommits: string;
    requestMetrics: string;
    serverUptime: string;
    authRequests: string;
    writeRequests: string;
    readAuthRequests: string;
    readUnauthRequests: string;
    reqPerMin: string;
    refresh: string;
  };
}
