// E2E tests for language switching in settings.

import { test, expect, registerUser } from './helpers';

test.describe('Language Settings', () => {
  test('changing language updates settings page labels immediately', async ({ page, request }) => {
    const { token } = await registerUser(request, 'lang-settings-page');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Open user menu and go to profile/settings
    const userMenu = page.locator('[data-testid="user-menu-button"]');
    await userMenu.click();

    const profileOption = page.locator('button', { hasText: /Profile/i });
    await expect(profileOption).toBeVisible({ timeout: 3000 });
    await profileOption.click();

    // Wait for settings page to load
    await expect(page).toHaveURL(/\/settings\/user/, { timeout: 5000 });

    // Verify we're in English - check the "Settings" title in the header
    // In French it becomes "Paramètres"
    const settingsTitle = page.locator('header h1');
    await expect(settingsTitle).toHaveText('Settings', { timeout: 5000 });

    // Also check the "Personal Settings" section header
    // In French: "Paramètres personnels"
    const personalSettingsHeader = page.locator('h3', { hasText: 'Personal Settings' });
    await expect(personalSettingsHeader).toBeVisible({ timeout: 5000 });

    // Find the language dropdown and change to French
    const languageSelect = page.locator('select').filter({ has: page.locator('option[value="fr"]') });
    await expect(languageSelect).toBeVisible({ timeout: 5000 });
    await languageSelect.selectOption('fr');

    // Click Save button (use specific text to avoid matching password form button)
    const saveButton = page.getByRole('button', { name: 'Save Changes' });
    await saveButton.click();

    // Wait for success message
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });

    // The key test: Settings page labels should update immediately WITHOUT navigating away
    // "Settings" -> "Paramètres"
    await expect(settingsTitle).toHaveText('Paramètres', { timeout: 5000 });

    // "Personal Settings" -> "Paramètres personnels"
    const personalSettingsHeaderFr = page.locator('h3', { hasText: 'Paramètres personnels' });
    await expect(personalSettingsHeaderFr).toBeVisible({ timeout: 5000 });
  });

  test('changing language in settings updates UI without refresh', async ({ page, request }) => {
    const { token } = await registerUser(request, 'lang-switch');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Verify we start in English - check sidebar "Create Workspace" button text
    // (in French it's "Créer un espace de travail")
    const createWsButtonEn = page.locator('aside button', { hasText: 'Create Workspace' });
    await expect(createWsButtonEn).toBeVisible({ timeout: 5000 });

    // Open user menu and go to profile/settings
    const userMenu = page.locator('[data-testid="user-menu-button"]');
    await userMenu.click();

    const profileOption = page.locator('button', { hasText: /Profile/i });
    await expect(profileOption).toBeVisible({ timeout: 3000 });
    await profileOption.click();

    // Wait for settings page to load
    await expect(page).toHaveURL(/\/settings\/user/, { timeout: 5000 });

    // Find the language dropdown and change to French
    const languageSelect = page.locator('select').filter({ has: page.locator('option[value="fr"]') });
    await expect(languageSelect).toBeVisible({ timeout: 5000 });

    // Verify current value is English
    await expect(languageSelect).toHaveValue('en');

    // Change to French
    await languageSelect.selectOption('fr');

    // Click Save button (use specific text to avoid matching password form button)
    const saveButton = page.getByRole('button', { name: 'Save Changes' });
    await saveButton.click();

    // Wait for success message to appear
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });

    // Navigate back to workspace using the specific back button in header
    // Use a more specific selector to avoid matching other buttons
    const backButton = page.locator('header button', { hasText: /←/ });
    await backButton.click();

    // Wait for sidebar to be visible
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // The key test: UI should show French without a page refresh
    // Create workspace button should now be in French
    const createWsButtonFr = page.locator('aside button', { hasText: 'Créer un espace de travail' });
    await expect(createWsButtonFr).toBeVisible({ timeout: 5000 });
  });

  test('language persists after page reload', async ({ page, request }) => {
    const { token } = await registerUser(request, 'lang-persist');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // Verify we start in English - check Create Workspace button text
    const createWsButtonEn = page.locator('aside button', { hasText: 'Create Workspace' });
    await expect(createWsButtonEn).toBeVisible({ timeout: 5000 });

    // Open settings and change to French
    const userMenu = page.locator('[data-testid="user-menu-button"]');
    await userMenu.click();

    const profileOption = page.locator('button', { hasText: /Profile/i });
    await expect(profileOption).toBeVisible({ timeout: 3000 });
    await profileOption.click();

    await expect(page).toHaveURL(/\/settings\/user/, { timeout: 5000 });

    const languageSelect = page.locator('select').filter({ has: page.locator('option[value="fr"]') });
    await expect(languageSelect).toBeVisible({ timeout: 5000 });
    await languageSelect.selectOption('fr');

    const saveButton = page.getByRole('button', { name: 'Save Changes' });
    await saveButton.click();

    // Wait for save to complete
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });

    // Navigate back to workspace first
    const backButton = page.locator('header button', { hasText: /←/ });
    await backButton.click();

    // Wait for sidebar to be visible
    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Reload the page
    await page.reload();

    // Wait for app to load
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // UI should be in French after reload - Create workspace button should be in French
    const createWsButtonFr = page.locator('aside button', { hasText: 'Créer un espace de travail' });
    await expect(createWsButtonFr).toBeVisible({ timeout: 5000 });
  });

  test('switching back to English works', async ({ page, request }) => {
    const { token } = await registerUser(request, 'lang-switch-back');
    await page.goto(`/?token=${token}`);
    await expect(page.locator('aside')).toBeVisible({ timeout: 15000 });

    // First change to French
    const userMenu = page.locator('[data-testid="user-menu-button"]');
    await userMenu.click();

    const profileOption = page.locator('button', { hasText: /Profile/i });
    await expect(profileOption).toBeVisible({ timeout: 3000 });
    await profileOption.click();

    await expect(page).toHaveURL(/\/settings\/user/, { timeout: 5000 });

    let languageSelect = page.locator('select').filter({ has: page.locator('option[value="fr"]') });
    await expect(languageSelect).toBeVisible({ timeout: 5000 });
    await languageSelect.selectOption('fr');

    let saveButton = page.getByRole('button', { name: 'Save Changes' });
    await saveButton.click();
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });

    // Now change back to English (UI is now in French, so button text is French)
    languageSelect = page.locator('select').filter({ has: page.locator('option[value="en"]') });
    await languageSelect.selectOption('en');

    saveButton = page.getByRole('button', { name: 'Enregistrer les modifications' });
    await saveButton.click();
    await expect(page.locator('[class*="success"]')).toBeVisible({ timeout: 5000 });

    // Navigate back using the specific back button
    const backButton = page.locator('header button', { hasText: /←/ });
    await backButton.click();

    await expect(page.locator('aside')).toBeVisible({ timeout: 10000 });

    // Should be back to English - Create workspace button should be in English
    const createWsButtonEn = page.locator('aside button', { hasText: 'Create Workspace' });
    await expect(createWsButtonEn).toBeVisible({ timeout: 5000 });
  });
});
