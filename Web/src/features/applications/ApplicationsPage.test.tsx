import { render, screen } from '@testing-library/react'
import { I18nextProvider } from 'react-i18next'
import { MemoryRouter } from 'react-router'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import i18n from '@/shared/i18n/config'
import linkStyles from '@/shared/link/ThemedLink.module.css'

import { ApplicationsPage } from './ApplicationsPage'

const api = vi.hoisted(() => ({
  applications: vi.fn(),
}))

vi.mock('@/generated/api', () => ({
  confirmApplicationOnboarding: vi.fn(),
  useDryRunApplicationOnboarding: vi.fn(),
  useGetCatalogApplication: vi.fn(),
  useGetCatalogApplicationStatus: vi.fn(),
  useListVisibleCatalogApplications: api.applications,
}))

describe('ApplicationsPage', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('zh-TW')
    api.applications.mockReset()
  })

  it('shows visible applications and links to the detail route', () => {
    api.applications.mockReturnValue({
      data: {
        status: 200,
        data: {
          data: [
            {
              argocdApplicationName: 'payment-production',
              argocdProject: 'payment',
              id: 'application-1',
              name: 'Payment',
              sourceTargetRevision: 'main',
            },
          ],
        },
      },
      isError: false,
      isPending: false,
    })

    renderPage()

    expect(screen.getByRole('link', { name: 'Payment' })).toHaveAttribute(
      'href',
      '/applications/application-1',
    )
    expect(screen.getByRole('link', { name: 'Payment' })).toHaveClass(
      linkStyles.link,
    )
    expect(screen.getByText('payment-production')).toBeInTheDocument()
  })

  it('preserves the unavailable state', () => {
    api.applications.mockReturnValue({
      data: undefined,
      isError: true,
      isPending: false,
    })

    renderPage()

    expect(screen.getByRole('alert')).toBeInTheDocument()
  })
})

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <MemoryRouter>
        <ApplicationsPage />
      </MemoryRouter>
    </I18nextProvider>,
  )
}
