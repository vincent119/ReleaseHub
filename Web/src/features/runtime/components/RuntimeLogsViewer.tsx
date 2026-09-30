import { Checkbox, Empty, Flex, Radio, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import type { RuntimeLogEntry } from '@/generated/model'
import { formatLogText, type LogDisplayMode } from '../model/formatLogText'
import styles from './RuntimeLogsViewer.module.css'

export function RuntimeLogsViewer({ entries }: { entries: RuntimeLogEntry[] }) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<LogDisplayMode>('readable')
  const [showTime, setShowTime] = useState(true)
  const [wrap, setWrap] = useState(false)
  const rows = useMemo(
    () =>
      entries.map((entry) => ({
        timestamp: formatLogText(entry.timestamp, mode),
        content: formatLogText(entry.content, mode),
      })),
    [entries, mode],
  )
  if (entries.length === 0)
    return <Empty description={t('runtimeTopology.logsViewer.empty')} />
  return (
    <section
      className={styles.viewer}
      aria-label={t('runtimeTopology.tabs.logs')}
    >
      <Flex gap="middle" align="center" wrap className={styles.toolbar}>
        <Radio.Group
          aria-label={t('runtimeTopology.logsViewer.mode')}
          value={mode}
          onChange={(event) =>
            setMode(event.target.value === 'raw' ? 'raw' : 'readable')
          }
          optionType="button"
          options={[
            {
              value: 'readable',
              label: t('runtimeTopology.logsViewer.readable'),
            },
            { value: 'raw', label: t('runtimeTopology.logsViewer.raw') },
          ]}
        />
        <Checkbox
          checked={showTime}
          onChange={(event) => setShowTime(event.target.checked)}
        >
          {t('runtimeTopology.logsViewer.showTime')}
        </Checkbox>
        <Checkbox
          checked={wrap}
          onChange={(event) => setWrap(event.target.checked)}
        >
          {t('runtimeTopology.logsViewer.wrap')}
        </Checkbox>
      </Flex>
      {mode === 'raw' && (
        <Typography.Text type="secondary" role="status">
          {t('runtimeTopology.logsViewer.rawHint')}
        </Typography.Text>
      )}
      <div
        className={`${styles.content} ${wrap ? styles.wrap : ''}`}
        tabIndex={0}
        role="region"
        aria-label={t('runtimeTopology.logsViewer.content')}
      >
        <table
          className={styles.table}
          aria-label={t('runtimeTopology.logsViewer.records')}
        >
          <thead>
            <tr>
              {showTime && (
                <th scope="col" className={styles.timestamp}>
                  {t('runtimeTopology.logsViewer.timestamp')}
                </th>
              )}
              <th scope="col">{t('runtimeTopology.logsViewer.message')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, index) => (
              <tr key={index}>
                {showTime && (
                  <td className={styles.timestamp}>
                    <code>{row.timestamp}</code>
                  </td>
                )}
                <td>
                  <pre className={styles.message}>{row.content}</pre>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  )
}
