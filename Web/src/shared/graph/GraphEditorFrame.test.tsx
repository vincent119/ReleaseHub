import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import { GraphEditorFrame } from './GraphEditorFrame'

afterEach(cleanup)

describe('GraphEditorFrame', () => {
  it('composes accessible toolbar, canvas, and inspector slots', () => {
    render(
      <GraphEditorFrame
        toolbarLabel="Graph tools"
        canvasLabel="Graph canvas"
        inspectorLabel="Graph inspector"
        selectionTitle="Selected node"
        toolbar={<button type="button">Add node</button>}
        canvas={<div>Canvas content</div>}
        inspector={<div>Inspector content</div>}
      />,
    )

    expect(
      screen.getByRole('toolbar', { name: 'Graph tools' }),
    ).toHaveTextContent('Add node')
    expect(screen.getByLabelText('Graph canvas')).toHaveTextContent(
      'Canvas content',
    )
    expect(screen.getByLabelText('Graph inspector')).toHaveTextContent(
      'Selected node',
    )
    expect(screen.getByLabelText('Graph inspector')).toHaveTextContent(
      'Inspector content',
    )
  })

  it('keeps feature classes on each shared region', () => {
    render(
      <GraphEditorFrame
        className="feature-editor"
        toolbarClassName="feature-toolbar"
        canvasClassName="feature-canvas"
        inspectorClassName="feature-inspector"
        toolbarLabel="Tools"
        canvasLabel="Canvas"
        inspectorLabel="Inspector"
        selectionTitle="Nothing selected"
        toolbar={null}
        canvas={null}
        inspector={null}
      />,
    )

    expect(screen.getByRole('toolbar', { name: 'Tools' })).toHaveClass(
      'feature-toolbar',
    )
    expect(screen.getByLabelText('Canvas')).toHaveClass('feature-canvas')
    expect(screen.getByLabelText('Inspector')).toHaveClass('feature-inspector')
    expect(
      screen.getByRole('toolbar', { name: 'Tools' }).parentElement,
    ).toHaveClass('feature-editor')
  })
})
