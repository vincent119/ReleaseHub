import { describe, expect, it } from 'vitest'

import { resolveResourceScope, updateResourceScope } from './resourceScope'
import { multipleScopeFixture, scopeFixture } from './scopeFixtures'

describe('範圍選擇的 ID 與相依關係', () => {
  it('單一組織以 ID 帶入，不自動選取 Project', () => {
    expect(resolveResourceScope(scopeFixture()).value).toEqual({
      organizationId: 'organization-a',
    })
  })

  it('多組織不猜第一筆，選項依指定組織隔離', () => {
    const organizations = multipleScopeFixture()
    expect(resolveResourceScope(organizations).value).toEqual({})
    const scope = resolveResourceScope(organizations, {
      organizationId: 'organization-b',
      projectId: 'project-0',
    })
    expect(scope.value).toEqual({ organizationId: 'organization-b' })
    expect(scope.organization?.projects.map((project) => project.id)).toEqual([
      'project-b',
    ])
  })

  it('相同名稱不影響 ID，Global 維持實際環境', () => {
    expect(
      updateResourceScope(
        scopeFixture(),
        { projectId: 'project-0' },
        'environmentId',
        'global-0',
      ),
    ).toEqual({
      organizationId: 'organization-a',
      projectId: 'project-0',
      environmentId: 'global-0',
    })
  })

  it('切換組織清除 Project 與環境', () => {
    expect(
      updateResourceScope(
        multipleScopeFixture(),
        {
          organizationId: 'organization-a',
          projectId: 'project-0',
          environmentId: 'environment-0',
        },
        'organizationId',
        'organization-b',
      ),
    ).toEqual({ organizationId: 'organization-b' })
  })

  it('切換或清除 Project 都清除環境', () => {
    const value = { projectId: 'project-0', environmentId: 'environment-0' }
    expect(
      updateResourceScope(scopeFixture(), value, 'projectId', 'project-1'),
    ).toEqual({ organizationId: 'organization-a', projectId: 'project-1' })
    expect(
      updateResourceScope(scopeFixture(), value, 'projectId', undefined),
    ).toEqual({ organizationId: 'organization-a' })
  })

  it('清除環境保留父 scope，拒絕其他 Project 的環境', () => {
    const value = { projectId: 'project-0', environmentId: 'environment-0' }
    for (const environmentId of [undefined, 'environment-1']) {
      expect(
        updateResourceScope(
          scopeFixture(),
          value,
          'environmentId',
          environmentId,
        ),
      ).toEqual({ organizationId: 'organization-a', projectId: 'project-0' })
    }
  })

  it('失效組織不悄悄選取另一組織，空資料不造 ID', () => {
    expect(
      resolveResourceScope(scopeFixture(), {
        organizationId: 'organization-removed',
        projectId: 'project-0',
      }).value,
    ).toEqual({})
    expect(resolveResourceScope([]).value).toEqual({})
  })

  it('純 helper 不修改 Catalog 或呼叫端 value', () => {
    const organizations = scopeFixture()
    const value = { projectId: 'project-0', environmentId: 'environment-0' }
    const before = JSON.stringify({ organizations, value })
    updateResourceScope(organizations, value, 'projectId', 'project-1')
    expect(JSON.stringify({ organizations, value })).toBe(before)
  })
})
