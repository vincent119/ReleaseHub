import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { SemanticList, SemanticListItemContent } from './SemanticList'

describe('SemanticList', () => {
  afterEach(cleanup)

  it('renders native list semantics with metadata and actions', () => {
    render(
      <SemanticList
        items={[{ id: 'one', name: 'Primary item' }]}
        rowKey="id"
        ariaLabel="Items"
        renderItem={(item) => (
          <SemanticListItemContent
            title={item.name}
            description="Description"
            actions={[<button key="action">Open</button>]}
          />
        )}
      />,
    )

    expect(screen.getByRole('list', { name: 'Items' })).toBeInTheDocument()
    expect(screen.getByRole('listitem')).toHaveTextContent(
      'Primary itemDescriptionOpen',
    )
    expect(screen.getByRole('button', { name: 'Open' })).toBeInTheDocument()
  })

  it('forwards interactive item properties and renders empty content', () => {
    const onSelect = vi.fn()
    const { rerender } = render(
      <SemanticList
        items={[{ id: 'one', name: 'Primary item' }]}
        rowKey="id"
        itemProps={() => ({
          role: 'button',
          tabIndex: 0,
          'aria-current': 'page',
          onClick: onSelect,
          onKeyDown: (event) => {
            if (event.key === 'Enter') onSelect()
          },
        })}
        renderItem={(item) => <SemanticListItemContent title={item.name} />}
      />,
    )

    const item = screen.getByRole('button', { name: 'Primary item' })
    expect(item).toHaveAttribute('aria-current', 'page')
    fireEvent.keyDown(item, { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledOnce()

    rerender(
      <SemanticList
        items={[]}
        rowKey="id"
        emptyContent={<span>No items</span>}
        renderItem={() => null}
      />,
    )
    expect(screen.getByText('No items')).toBeInTheDocument()
  })

  it('exposes the list loading state without hiding its content', () => {
    render(
      <SemanticList
        items={[{ id: 'one', name: 'Primary item' }]}
        rowKey="id"
        loading
        renderItem={(item) => <SemanticListItemContent title={item.name} />}
      />,
    )

    expect(screen.getByRole('list')).toHaveAttribute('aria-busy', 'true')
    expect(screen.getByText('Primary item')).toBeInTheDocument()
    expect(document.querySelector('.ant-spin-spinning')).not.toBeNull()
  })
})
