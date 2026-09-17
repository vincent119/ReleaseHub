import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router'
import { describe, expect, it } from 'vitest'

import i18n from '@/shared/i18n/config'

import { navigationItems } from './navigationItems'

describe('English sidebar navigation', () => {
  it('uses concise visible labels and complete accessible names', async () => {
    await i18n.changeLanguage('en')
    const items = navigationItems(i18n.t)

    render(
      <MemoryRouter>
        {items.map((item) =>
          item && 'label' in item ? (
            <div key={item.key}>{item.label}</div>
          ) : null,
        )}
      </MemoryRouter>,
    )

    expect(
      screen.getByRole('link', { name: 'Deployment Requests' }),
    ).toHaveTextContent('Requests')
    expect(
      screen.getByRole('link', { name: 'Release Workflows' }),
    ).toHaveTextContent('Workflows')
    expect(
      screen.getByRole('link', { name: 'Deployment Plans' }),
    ).toHaveTextContent('Plans')
    expect(
      screen.getByRole('link', { name: 'Access management' }),
    ).toHaveTextContent('Access')
  })

  it('only includes Audit Trail when the server capability is visible', async () => {
    await i18n.changeLanguage('en')
    const hidden = navigationItems(i18n.t)
    const visible = navigationItems(i18n.t, true)

    expect(hidden.some((item) => item.key === '/audit')).toBe(false)
    expect(visible.some((item) => item.key === '/audit')).toBe(true)
  })
})
