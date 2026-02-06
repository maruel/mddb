// Admin dashboard showing server-wide stats, org/workspace breakdown, and request metrics.

import { createSignal, createEffect, For, Show } from 'solid-js';
import { useAuth } from '../../contexts';
import { useI18n } from '../../i18n';
import type { AdminServerDetail } from '@sdk/types.gen';
import styles from './AdminDashboard.module.css';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(1024));
  return `${(bytes / Math.pow(1024, i)).toFixed(i > 0 ? 1 : 0)} ${units[i]}`;
}

function formatUptime(seconds: number): string {
  const d = Math.floor(seconds / 86400);
  const h = Math.floor((seconds % 86400) / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = Math.floor(seconds % 60);
  if (d > 0) return `${d}d ${h}h ${m}m ${s}s`;
  if (h > 0) return `${h}h ${m}m ${s}s`;
  return `${m}m ${s}s`;
}

function formatDate(unixSeconds: number): string {
  return new Date(unixSeconds * 1000).toLocaleDateString();
}

function reqPerMin(count: number, uptimeSeconds: number): string {
  if (uptimeSeconds <= 0) return '0';
  return ((count / uptimeSeconds) * 60).toFixed(1);
}

export default function AdminDashboard() {
  const { t } = useI18n();
  const { api } = useAuth();

  const [data, setData] = createSignal<AdminServerDetail | null>(null);
  const [loading, setLoading] = createSignal(false);
  const [error, setError] = createSignal<string | null>(null);

  const load = async () => {
    try {
      setLoading(true);
      setError(null);
      const resp = await api().admin.getServerDetail();
      setData(resp);
    } catch (err) {
      setError(`${t('errors.failedToLoad')}: ${err}`);
    } finally {
      setLoading(false);
    }
  };

  createEffect(() => {
    load();
  });

  return (
    <div class={styles.dashboard}>
      <Show when={error()}>
        <div class={styles.error}>{error()}</div>
      </Show>

      <Show when={data()}>
        {(d) => {
          const metrics = () => d().request_metrics;
          return (
            <>
              {/* Summary cards */}
              <div class={styles.summaryCards}>
                <div class={styles.card}>
                  <div class={styles.cardValue}>{d().user_count}</div>
                  <div class={styles.cardLabel}>{t('server.totalUsers')}</div>
                </div>
                <div class={styles.card}>
                  <div class={styles.cardValue}>{d().org_count}</div>
                  <div class={styles.cardLabel}>{t('server.totalOrganizations')}</div>
                </div>
                <div class={styles.card}>
                  <div class={styles.cardValue}>{d().workspace_count}</div>
                  <div class={styles.cardLabel}>{t('server.totalWorkspaces')}</div>
                </div>
                <div class={styles.card}>
                  <div class={styles.cardValue}>{formatBytes(d().total_storage)}</div>
                  <div class={styles.cardLabel}>{t('server.totalStorage')}</div>
                </div>
                <div class={styles.card}>
                  <div class={styles.cardValue}>{d().active_sessions}</div>
                  <div class={styles.cardLabel}>{t('server.activeSessions')}</div>
                </div>
              </div>

              {/* Refresh */}
              <div class={styles.refreshRow}>
                <button class={styles.refreshButton} onClick={load} disabled={loading()}>
                  {t('server.refresh')}
                </button>
              </div>

              {/* Org/Workspace table */}
              <table class={styles.orgTable}>
                <thead>
                  <tr>
                    <th>
                      {t('server.organizationName')} / {t('server.workspaceName')}
                    </th>
                    <th class={styles.numCell}>{t('server.members')}</th>
                    <th class={styles.numCell}>{t('server.pages')}</th>
                    <th class={styles.numCell}>{t('server.storage')}</th>
                    <th class={styles.numCell}>{t('server.gitCommits')}</th>
                    <th>{t('server.created')}</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={d().organizations}>
                    {(org) => (
                      <>
                        <tr class={styles.orgHeader}>
                          <td>{org.name}</td>
                          <td class={styles.numCell}>{org.member_count}</td>
                          <td class={styles.numCell}>{org.workspace_count} ws</td>
                          <td />
                          <td />
                          <td>{formatDate(org.created)}</td>
                        </tr>
                        <For each={org.workspaces}>
                          {(ws) => (
                            <tr class={styles.wsRow}>
                              <td>{ws.name}</td>
                              <td class={styles.numCell}>{ws.member_count}</td>
                              <td class={styles.numCell}>{ws.page_count}</td>
                              <td class={styles.numCell}>{formatBytes(ws.storage_bytes)}</td>
                              <td class={styles.numCell}>{ws.git_commits}</td>
                              <td>{formatDate(ws.created)}</td>
                            </tr>
                          )}
                        </For>
                      </>
                    )}
                  </For>
                </tbody>
              </table>

              {/* Request metrics */}
              <div class={styles.metricsSection}>
                <h3>{t('server.requestMetrics')}</h3>
                <div class={styles.metricsGrid}>
                  <div class={styles.metric}>
                    <div class={styles.metricLabel}>{t('server.serverUptime')}</div>
                    <div class={styles.metricValue}>{formatUptime(metrics().uptime_seconds)}</div>
                  </div>
                  <div class={styles.metric}>
                    <div class={styles.metricLabel}>{t('server.authRequests')}</div>
                    <div class={styles.metricValue}>{metrics().auth_count.toLocaleString()}</div>
                    <div class={styles.metricRate}>
                      {reqPerMin(metrics().auth_count, metrics().uptime_seconds)} {t('server.reqPerMin')}
                    </div>
                  </div>
                  <div class={styles.metric}>
                    <div class={styles.metricLabel}>{t('server.writeRequests')}</div>
                    <div class={styles.metricValue}>{metrics().write_count.toLocaleString()}</div>
                    <div class={styles.metricRate}>
                      {reqPerMin(metrics().write_count, metrics().uptime_seconds)} {t('server.reqPerMin')}
                    </div>
                  </div>
                  <div class={styles.metric}>
                    <div class={styles.metricLabel}>{t('server.readAuthRequests')}</div>
                    <div class={styles.metricValue}>{metrics().read_auth_count.toLocaleString()}</div>
                    <div class={styles.metricRate}>
                      {reqPerMin(metrics().read_auth_count, metrics().uptime_seconds)} {t('server.reqPerMin')}
                    </div>
                  </div>
                  <div class={styles.metric}>
                    <div class={styles.metricLabel}>{t('server.readUnauthRequests')}</div>
                    <div class={styles.metricValue}>{metrics().read_unauth_count.toLocaleString()}</div>
                    <div class={styles.metricRate}>
                      {reqPerMin(metrics().read_unauth_count, metrics().uptime_seconds)} {t('server.reqPerMin')}
                    </div>
                  </div>
                </div>
              </div>
            </>
          );
        }}
      </Show>
    </div>
  );
}
