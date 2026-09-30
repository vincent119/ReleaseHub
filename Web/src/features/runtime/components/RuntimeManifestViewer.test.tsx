import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { RuntimeManifestViewer } from './RuntimeManifestViewer'

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))
afterEach(cleanup)
const label = (name: string) => `runtimeTopology.manifestViewer.${name}`

describe('RuntimeManifestViewer', () => {
  it('switches filtered, complete and original modes without mutating the input', () => {
    const source =
      ' {"kind":"Secret","metadata":{"managedFields":[{}]},"data":{"token":"[REDACTED]"},"version":9007199254740993999} '
    render(<RuntimeManifestViewer manifest={source} />)
    const content = screen.getByLabelText(label('content'))
    expect(content.textContent).toContain('\n  "kind": "Secret",')
    expect(content.textContent).not.toContain('managedFields')
    expect(screen.getByRole('status')).toHaveTextContent(label('hidden'))
    fireEvent.click(
      screen.getByRole('checkbox', { name: label('showManagedFields') }),
    )
    expect(content.textContent).toContain('"managedFields": [')
    expect(content.textContent).toContain('9007199254740993999')
    expect(screen.queryByRole('status')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('radio', { name: label('raw') }))
    expect(content.textContent).toBe(source)
    expect(
      screen.queryByRole('checkbox', { name: label('showManagedFields') }),
    ).not.toBeInTheDocument()
    expect(content.textContent).toContain('[REDACTED]')
    fireEvent.click(screen.getByRole('checkbox', { name: label('wrap') }))
    expect(screen.getByRole('checkbox', { name: label('wrap') })).toBeChecked()
    expect(content.textContent).toBe(source)
  })

  it('shows invalid text safely rather than executing markup', () => {
    const source = '<img src=x onerror=alert(1)>'
    const { container } = render(<RuntimeManifestViewer manifest={source} />)
    expect(screen.getByRole('alert')).toHaveTextContent(label('invalid'))
    expect(screen.getByLabelText(label('content')).textContent).toBe(source)
    expect(container.querySelector('img')).toBeNull()
    expect(screen.queryByRole('radio')).not.toBeInTheDocument()
  })

  it('shows empty state distinctly from parse failures', () => {
    render(<RuntimeManifestViewer manifest="" />)
    expect(screen.getByText(label('empty'))).toBeInTheDocument()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('preserves the original in a bounded formatting fallback', () => {
    const source = '['.repeat(70) + '0' + ']'.repeat(70)
    render(<RuntimeManifestViewer manifest={source} />)
    expect(screen.getByRole('alert')).toHaveTextContent(label('limited'))
    expect(screen.getByLabelText(label('content')).textContent).toBe(source)
  })
})
