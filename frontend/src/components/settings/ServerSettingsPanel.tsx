// Server settings panel for global admins: dashboard, SMTP configuration, and quotas/rate limits.

import { createSignal, createEffect, Show, lazy } from 'solid-js';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import type { ServerConfigResponse } from '@sdk/types.gen';
import styles from './ServerSettingsPanel.module.css';

const AdminDashboard = lazy(() => import('./AdminDashboard'));

type Tab = 'dashboard' | 'smtp' | 'quotas';

export default function ServerSettingsPanel() {
  const { t } = useI18n();
  const { api } = useAuth();

  const [activeTab, setActiveTab] = createSignal<Tab>('dashboard');
  const [config, setConfig] = createSignal<ServerConfigResponse | null>(null);

  // SMTP fields
  const [smtpHost, setSmtpHost] = createSignal('');
  const [smtpPort, setSmtpPort] = createSignal(587);
  const [smtpUsername, setSmtpUsername] = createSignal('');
  const [smtpPassword, setSmtpPassword] = createSignal('');
  const [smtpFrom, setSmtpFrom] = createSignal('');

  // Quota fields
  const [maxRequestBodyBytes, setMaxRequestBodyBytes] = createSignal(0);
  const [maxSessionsPerUser, setMaxSessionsPerUser] = createSignal(0);
  const [maxTablesPerWorkspace, setMaxTablesPerWorkspace] = createSignal(0);
  const [maxColumnsPerTable, setMaxColumnsPerTable] = createSignal(0);
  const [maxRecordsPerTable, setMaxRecordsPerTable] = createSignal(0);
  const [maxPages, setMaxPages] = createSignal(0);
  const [maxStorageBytes, setMaxStorageBytes] = createSignal(0);
  const [maxOrganizations, setMaxOrganizations] = createSignal(0);
  const [maxWorkspaces, setMaxWorkspaces] = createSignal(0);
  const [maxUsers, setMaxUsers] = createSignal(0);
  const [maxTotalStorageBytes, setMaxTotalStorageBytes] = createSignal(0);
  const [maxAssetSizeBytes, setMaxAssetSizeBytes] = createSignal(0);
  const [maxEgressBandwidthBps, setMaxEgressBandwidthBps] = createSignal(0);

  // Rate limit fields
  const [authRatePerMin, setAuthRatePerMin] = createSignal(0);
  const [writeRatePerMin, setWriteRatePerMin] = createSignal(0);
  const [readAuthRatePerMin, setReadAuthRatePerMin] = createSignal(0);
  const [readUnauthRatePerMin, setReadUnauthRatePerMin] = createSignal(0);

  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);
  const [success, setSuccess] = createSignal<string | null>(null);

  const loadConfig = async () => {
    try {
      setLoading(true);
      setError(null);
      const data = await api().server.getConfig();
      setConfig(data);

      // Populate SMTP fields
      setSmtpHost(data.smtp.host || '');
      setSmtpPort(data.smtp.port || 587);
      setSmtpUsername(data.smtp.username || '');
      setSmtpFrom(data.smtp.from || '');
      setSmtpPassword(''); // Always empty on load

      // Populate quota fields
      setMaxRequestBodyBytes(data.quotas.max_request_body_bytes);
      setMaxSessionsPerUser(data.quotas.max_sessions_per_user);
      setMaxTablesPerWorkspace(data.quotas.max_tables_per_workspace);
      setMaxColumnsPerTable(data.quotas.max_columns_per_table);
      setMaxRecordsPerTable(data.quotas.max_records_per_table);
      setMaxPages(data.quotas.max_pages);
      setMaxStorageBytes(data.quotas.max_storage_bytes);
      setMaxOrganizations(data.quotas.max_organizations);
      setMaxWorkspaces(data.quotas.max_workspaces);
      setMaxUsers(data.quotas.max_users);
      setMaxTotalStorageBytes(data.quotas.max_total_storage_bytes);
      setMaxAssetSizeBytes(data.quotas.max_asset_size_bytes);
      setMaxEgressBandwidthBps(data.quotas.max_egress_bandwidth_bps);

      // Populate rate limit fields
      setAuthRatePerMin(data.rate_limits.auth_rate_per_min);
      setWriteRatePerMin(data.rate_limits.write_rate_per_min);
      setReadAuthRatePerMin(data.rate_limits.read_auth_rate_per_min);
      setReadUnauthRatePerMin(data.rate_limits.read_unauth_rate_per_min);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  createEffect(() => {
    loadConfig();
  });

  const saveSMTP = async (e: Event) => {
    e.preventDefault();
    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      await api().server.updateConfig({
        smtp: {
          host: smtpHost(),
          port: smtpPort(),
          username: smtpUsername(),
          password: smtpPassword(), // Empty preserves existing
          from: smtpFrom(),
        },
      });

      setSuccess(t('server.configurationSaved'));
      // Reload to get updated state
      await loadConfig();
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  const saveQuotas = async (e: Event) => {
    e.preventDefault();
    try {
      setLoading(true);
      setError(null);
      setSuccess(null);

      await api().server.updateConfig({
        quotas: {
          max_request_body_bytes: maxRequestBodyBytes(),
          max_sessions_per_user: maxSessionsPerUser(),
          max_tables_per_workspace: maxTablesPerWorkspace(),
          max_columns_per_table: maxColumnsPerTable(),
          max_records_per_table: maxRecordsPerTable(),
          max_pages: maxPages(),
          max_storage_bytes: maxStorageBytes(),
          max_organizations: maxOrganizations(),
          max_workspaces: maxWorkspaces(),
          max_users: maxUsers(),
          max_total_storage_bytes: maxTotalStorageBytes(),
          max_asset_size_bytes: maxAssetSizeBytes(),
          max_egress_bandwidth_bps: maxEgressBandwidthBps(),
        },
        rate_limits: {
          auth_rate_per_min: authRatePerMin(),
          write_rate_per_min: writeRatePerMin(),
          read_auth_rate_per_min: readAuthRatePerMin(),
          read_unauth_rate_per_min: readUnauthRatePerMin(),
        },
      });

      setSuccess(t('server.configurationSaved'));
    } catch (err) {
      setError(`${t('errors.failedToSave')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div class={styles.panel}>
      <h2>{t('server.serverSettings')}</h2>

      <div class={styles.tabs}>
        <button class={activeTab() === 'dashboard' ? styles.activeTab : ''} onClick={() => setActiveTab('dashboard')}>
          {t('server.dashboard')}
        </button>
        <button class={activeTab() === 'smtp' ? styles.activeTab : ''} onClick={() => setActiveTab('smtp')}>
          {t('server.smtpConfiguration')}
        </button>
        <button class={activeTab() === 'quotas' ? styles.activeTab : ''} onClick={() => setActiveTab('quotas')}>
          {t('server.quotas')}
        </button>
      </div>

      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>
      <Show when={success()}>
        <div class={styles.success}>{success()}</div>
      </Show>

      <Show when={activeTab() === 'dashboard'}>
        <AdminDashboard />
      </Show>

      <Show when={activeTab() === 'smtp'}>
        <section class={styles.section}>
          <div class={styles.statusBadge}>
            <Show when={config()?.smtp.host} fallback={<span class={styles.disabled}>{t('server.smtpDisabled')}</span>}>
              <span class={styles.enabled}>{t('server.smtpEnabled')}</span>
            </Show>
          </div>

          <form onSubmit={saveSMTP} class={styles.settingsForm}>
            <div class={styles.formItem}>
              <label>{t('server.smtpHost')}</label>
              <input
                type="text"
                value={smtpHost()}
                onInput={(e) => setSmtpHost(e.target.value)}
                placeholder="smtp.example.com"
              />
            </div>

            <div class={styles.formItem}>
              <label>{t('server.smtpPort')}</label>
              <input
                type="number"
                value={smtpPort()}
                onInput={(e) => setSmtpPort(parseInt(e.target.value) || 587)}
                min="1"
                max="65535"
              />
            </div>

            <div class={styles.formItem}>
              <label>{t('server.smtpUsername')}</label>
              <input
                type="text"
                value={smtpUsername()}
                onInput={(e) => setSmtpUsername(e.target.value)}
                placeholder="user@example.com"
              />
            </div>

            <div class={styles.formItem}>
              <label>{t('server.smtpPassword')}</label>
              <input
                type="password"
                value={smtpPassword()}
                onInput={(e) => setSmtpPassword(e.target.value)}
                placeholder="••••••••"
              />
              <p class={styles.hint}>{t('server.smtpPasswordHint')}</p>
            </div>

            <div class={styles.formItem}>
              <label>{t('server.smtpFrom')}</label>
              <input
                type="email"
                value={smtpFrom()}
                onInput={(e) => setSmtpFrom(e.target.value)}
                placeholder="noreply@example.com"
              />
            </div>

            <button type="submit" class={styles.saveButton} disabled={loading()}>
              {t('server.saveConfiguration')}
            </button>
          </form>
        </section>
      </Show>

      <Show when={activeTab() === 'quotas'}>
        <section class={styles.section}>
          <form onSubmit={saveQuotas} class={styles.settingsForm}>
            <div class={styles.formGrid}>
              <div class={styles.formItem}>
                <label>{t('server.maxRequestBodyBytes')}</label>
                <input
                  type="number"
                  value={maxRequestBodyBytes()}
                  onInput={(e) => setMaxRequestBodyBytes(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxSessionsPerUser')}</label>
                <input
                  type="number"
                  value={maxSessionsPerUser()}
                  onInput={(e) => setMaxSessionsPerUser(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxTablesPerWorkspace')}</label>
                <input
                  type="number"
                  value={maxTablesPerWorkspace()}
                  onInput={(e) => setMaxTablesPerWorkspace(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxColumnsPerTable')}</label>
                <input
                  type="number"
                  value={maxColumnsPerTable()}
                  onInput={(e) => setMaxColumnsPerTable(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxRecordsPerTable')}</label>
                <input
                  type="number"
                  value={maxRecordsPerTable()}
                  onInput={(e) => setMaxRecordsPerTable(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxPages')}</label>
                <input
                  type="number"
                  value={maxPages()}
                  onInput={(e) => setMaxPages(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxStorageBytes')}</label>
                <input
                  type="number"
                  value={maxStorageBytes()}
                  onInput={(e) => setMaxStorageBytes(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxOrganizations')}</label>
                <input
                  type="number"
                  value={maxOrganizations()}
                  onInput={(e) => setMaxOrganizations(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxWorkspaces')}</label>
                <input
                  type="number"
                  value={maxWorkspaces()}
                  onInput={(e) => setMaxWorkspaces(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxUsers')}</label>
                <input
                  type="number"
                  value={maxUsers()}
                  onInput={(e) => setMaxUsers(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxTotalStorageBytes')}</label>
                <input
                  type="number"
                  value={maxTotalStorageBytes()}
                  onInput={(e) => setMaxTotalStorageBytes(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxAssetSizeBytes')}</label>
                <input
                  type="number"
                  value={maxAssetSizeBytes()}
                  onInput={(e) => setMaxAssetSizeBytes(parseInt(e.target.value) || 0)}
                  min="1"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.maxEgressBandwidthBps')}</label>
                <input
                  type="number"
                  value={maxEgressBandwidthBps()}
                  onInput={(e) => setMaxEgressBandwidthBps(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>
            </div>

            <h3>{t('server.rateLimits')}</h3>
            <p class={styles.hint}>{t('server.rateLimitsHint')}</p>
            <div class={styles.formGrid}>
              <div class={styles.formItem}>
                <label>{t('server.authRatePerMin')}</label>
                <input
                  type="number"
                  value={authRatePerMin()}
                  onInput={(e) => setAuthRatePerMin(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.writeRatePerMin')}</label>
                <input
                  type="number"
                  value={writeRatePerMin()}
                  onInput={(e) => setWriteRatePerMin(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.readAuthRatePerMin')}</label>
                <input
                  type="number"
                  value={readAuthRatePerMin()}
                  onInput={(e) => setReadAuthRatePerMin(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>

              <div class={styles.formItem}>
                <label>{t('server.readUnauthRatePerMin')}</label>
                <input
                  type="number"
                  value={readUnauthRatePerMin()}
                  onInput={(e) => setReadUnauthRatePerMin(parseInt(e.target.value) || 0)}
                  min="0"
                />
              </div>
            </div>

            <button type="submit" class={styles.saveButton} disabled={loading()}>
              {t('server.saveConfiguration')}
            </button>
          </form>
        </section>
      </Show>
    </div>
  );
}
