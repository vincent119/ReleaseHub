import { Alert, Checkbox, Empty, Flex, Radio, Typography } from 'antd'
import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { formatManifest } from '../model/formatManifest'
import styles from './RuntimeManifestViewer.module.css'

export function RuntimeManifestViewer({ manifest }: { manifest: string }) {
  const { t } = useTranslation()
  const [raw, setRaw] = useState(false)
  const [showManagedFields, setShowManagedFields] = useState(false)
  const [wrap, setWrap] = useState(false)
  const result = useMemo(
    () => formatManifest(manifest, showManagedFields),
    [manifest, showManagedFields],
  )
  const formatted = result.status === 'formatted'
  if (result.status === 'empty') {
    return <Empty description={t('runtimeTopology.manifestViewer.empty')} />
  }
  return (
    <section
      className={styles.viewer}
      aria-label={t('runtimeTopology.tabs.manifest')}
    >
      <Flex className={styles.toolbar} gap="middle" wrap align="center">
        {formatted && (
          <Radio.Group
            aria-label={t('runtimeTopology.manifestViewer.mode')}
            value={raw ? 'raw' : 'formatted'}
            onChange={(event) => setRaw(event.target.value === 'raw')}
            optionType="button"
            options={[
              {
                value: 'formatted',
                label: t('runtimeTopology.manifestViewer.formatted'),
              },
              { value: 'raw', label: t('runtimeTopology.manifestViewer.raw') },
            ]}
          />
        )}
        {formatted && !raw && result.hasManagedFields && (
          <Checkbox
            checked={showManagedFields}
            onChange={(event) => setShowManagedFields(event.target.checked)}
          >
            {t('runtimeTopology.manifestViewer.showManagedFields')}
          </Checkbox>
        )}
        <Checkbox
          checked={wrap}
          onChange={(event) => setWrap(event.target.checked)}
        >
          {t('runtimeTopology.manifestViewer.wrap')}
        </Checkbox>
      </Flex>
      {!formatted && (
        <Alert
          showIcon
          type="warning"
          title={t(`runtimeTopology.manifestViewer.${result.status}`)}
        />
      )}
      {formatted && !raw && result.hasManagedFields && !showManagedFields && (
        <Typography.Text type="secondary" role="status">
          {t('runtimeTopology.manifestViewer.hidden')}
        </Typography.Text>
      )}
      <pre
        className={`${styles.code} ${wrap ? styles.wrap : ''}`}
        tabIndex={0}
        aria-label={t('runtimeTopology.manifestViewer.content')}
      >
        {raw ? manifest : result.text}
      </pre>
    </section>
  )
}
