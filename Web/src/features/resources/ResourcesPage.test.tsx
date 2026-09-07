import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'

import { ResourcesPage } from './ResourcesPage'

const api = vi.hoisted(() => ({ useResources: vi.fn(), useSystem: vi.fn() }))

vi.mock('@/generated/api', () => ({
  useGetCatalogResourceTree: api.useResources,
  useGetSystemStatus: api.useSystem,
}))

describe('ResourcesPage', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    Object.defineProperty(window, 'matchMedia', {
      configurable: true,
      value: () => ({
        matches: false,
        addEventListener: () => undefined,
        removeEventListener: () => undefined,
      }),
    })
    api.useSystem.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        status: 200,
        data: {
          data: { name: 'ReleaseHub', version: 'test', tenancyMode: 'single' },
        },
      },
    })
    api.useResources.mockReturnValue({
      isPending: false,
      isError: false,
      data: {
        status: 200,
        data: {
          data: [
            {
              id: '019c1230-0000-7000-8000-000000000001',
              name: 'default',
              projects: [
                {
                  id: '019c1230-0000-7000-8000-000000000002',
                  name: 'Payment',
                  environments: [],
                },
              ],
            },
          ],
        },
      },
    })
  })

  it('hides the Organization presentation layer in single-tenant mode', () => {
    render(
      <MemoryRouter>
        <I18nextProvider i18n={i18n}>
          <ResourcesPage />
        </I18nextProvider>
      </MemoryRouter>,
    )

    expect(screen.getByText('Project: Payment')).toBeInTheDocument()
    expect(screen.queryByText('Organization: default')).not.toBeInTheDocument()
  })
})
