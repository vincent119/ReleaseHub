import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RuntimeLogsViewer } from './RuntimeLogsViewer'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
afterEach(cleanup)
const label = (name: string) => `runtimeTopology.logsViewer.${name}`

describe('RuntimeLogsViewer', () => {
  it('preserves entry order, embedded time, multiline content and blank entries', () => {
    const entries = [
      {
        timestamp: '2026-02-02T00:00:00Z',
        content:
          '\u001b[34mINFO\u001b[0m 2026-02-02T08:00:00+08:00 {"key":"value"}\n\tat file.go:1',
        podName: 'pod',
      },
      { timestamp: '2026-01-01T00:00:00Z', content: '', podName: 'pod' },
      {
        timestamp: '2026-01-01T00:00:00Z',
        content: 'duplicate timestamp',
        podName: 'pod',
      },
    ]
    const original = JSON.stringify(entries)
    const { container } = render(<RuntimeLogsViewer entries={entries} />)
    const rows = screen.getAllByRole('row').slice(1)
    expect(rows).toHaveLength(3)
    expect(within(rows[0]).getAllByRole('cell')[0]).toHaveTextContent(
      entries[0].timestamp,
    )
    expect(rows[0].querySelector('pre')?.textContent).toBe(
      'INFO 2026-02-02T08:00:00+08:00 {"key":"value"}\n\tat file.go:1',
    )
    expect(rows[1].querySelector('pre')?.textContent).toBe('')
    fireEvent.click(screen.getByRole('checkbox', { name: label('showTime') }))
    expect(within(rows[0]).getAllByRole('cell')).toHaveLength(1)
    expect(rows[0].textContent).toContain('2026-02-02T08:00:00+08:00')
    fireEvent.click(screen.getByRole('radio', { name: label('raw') }))
    expect(container.querySelector('pre')?.textContent).toBe(
      JSON.stringify(entries[0].content),
    )
    expect(screen.getByRole('status')).toHaveTextContent(label('rawHint'))
    fireEvent.click(screen.getByRole('checkbox', { name: label('wrap') }))
    expect(screen.getByRole('checkbox', { name: label('wrap') })).toBeChecked()
    expect(JSON.stringify(entries)).toBe(original)
  })

  it('keeps empty logs distinct from a blank entry', () => {
    const { rerender } = render(<RuntimeLogsViewer entries={[]} />)
    expect(screen.getByText(label('empty'))).toBeInTheDocument()
    expect(screen.queryByRole('table')).not.toBeInTheDocument()
    rerender(
      <RuntimeLogsViewer
        entries={[{ timestamp: '', content: '', podName: 'pod' }]}
      />,
    )
    expect(screen.queryByText(label('empty'))).not.toBeInTheDocument()
    expect(screen.getAllByRole('row')).toHaveLength(2)
  })

  it('never renders HTML or OSC links as active content in either mode', () => {
    const source =
      '\u001b]8;;https://example.invalid\u0007label\u001b]8;;\u0007 <img src=x onerror=alert(1)>'
    const { container } = render(
      <RuntimeLogsViewer
        entries={[{ timestamp: 'time', content: source, podName: 'pod' }]}
      />,
    )
    expect(container.querySelector('pre')?.textContent).toBe(
      'label <img src=x onerror=alert(1)>',
    )
    expect(container.querySelector('a, img, script')).toBeNull()
    fireEvent.click(screen.getByRole('radio', { name: label('raw') }))
    expect(JSON.parse(container.querySelector('pre')?.textContent ?? '')).toBe(
      source,
    )
    expect(container.querySelector('a, img, script')).toBeNull()
  })

  it('renders the existing 500-entry bound without dropping records', () => {
    render(
      <RuntimeLogsViewer
        entries={Array.from({ length: 500 }, (_, index) => ({
          timestamp: `${index}`,
          content: `record ${index}`,
          podName: 'pod',
        }))}
      />,
    )
    expect(screen.getAllByRole('row')).toHaveLength(501)
    expect(screen.getByText('record 499')).toBeInTheDocument()
  })
})
