import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'

import styles from './SemanticTag.module.css'
import { SemanticTag } from './SemanticTag'

describe('SemanticTag', () => {
  afterEach(cleanup)

  it('uses the neutral tone by default', () => {
    render(<SemanticTag>Production</SemanticTag>)

    expect(screen.getByText('Production')).toHaveClass(
      styles.root,
      styles.neutral,
    )
  })

  it.each(['success', 'warning', 'error', 'info'] as const)(
    'maps the %s tone to its semantic class',
    (tone) => {
      render(<SemanticTag tone={tone}>{tone}</SemanticTag>)

      expect(screen.getByText(tone)).toHaveClass(styles.root, styles[tone])
    },
  )

  it('preserves caller classes and native Tag properties', () => {
    render(
      <SemanticTag className="feature-tag" aria-label="Environment type">
        Production
      </SemanticTag>,
    )

    expect(screen.getByLabelText('Environment type')).toHaveClass(
      styles.root,
      styles.neutral,
      'feature-tag',
    )
  })
})
