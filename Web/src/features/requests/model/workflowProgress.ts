import type {
  ReleaseWorkflowDocument,
  ReleaseWorkflowState,
} from '@/generated/model'

export interface WorkflowProgressStage {
  key: string
  title: string
  content: string
  stateKeys: string[]
}

interface ProgressCopy {
  resultTitle: string
  resultPending: string
}

export function buildWorkflowProgress(
  document: ReleaseWorkflowDocument,
  current: string | undefined,
  copy: ProgressCopy,
) {
  const states = new Map(document.states.map((state) => [state.key, state]))
  const currentKey =
    current && states.has(current) ? current : document.initialState
  const stages = walkProgress(document, states, currentKey, copy)
  const currentIndex = stages.findIndex((stage) =>
    stage.stateKeys.includes(currentKey),
  )
  if (currentIndex >= 0) return { stages, currentIndex }

  const currentState = states.get(currentKey)
  return currentState
    ? { stages: [stateStage(currentState)], currentIndex: 0 }
    : { stages: [], currentIndex: 0 }
}

function walkProgress(
  document: ReleaseWorkflowDocument,
  states: Map<string, ReleaseWorkflowState>,
  currentKey: string,
  copy: ProgressCopy,
) {
  const outgoing = transitionTargets(document)
  const stages: WorkflowProgressStage[] = []
  const visited = new Set<string>()
  let stateKey: string | undefined = document.initialState

  while (stateKey && !visited.has(stateKey)) {
    visited.add(stateKey)
    const state = states.get(stateKey)
    if (!state) break
    stages.push(stateStage(state))

    const targets: string[] = outgoing.get(stateKey) ?? []
    const terminalTargets = targets
      .map((key) => states.get(key))
      .filter(
        (target): target is ReleaseWorkflowState => target?.type === 'Terminal',
      )
    if (targets.length > 1 && terminalTargets.length === targets.length) {
      stages.push(terminalOutcomeStage(terminalTargets, currentKey, copy))
      break
    }
    if (targets.length === 1) {
      stateKey = targets[0]
      continue
    }
    stateKey = uniqueTargetToCurrent(targets, currentKey, outgoing)
  }
  return stages
}

function transitionTargets(document: ReleaseWorkflowDocument) {
  const outgoing = new Map<string, string[]>()
  document.transitions.forEach((transition) => {
    const targets = outgoing.get(transition.from) ?? []
    if (!targets.includes(transition.to)) targets.push(transition.to)
    outgoing.set(transition.from, targets)
  })
  return outgoing
}

function uniqueTargetToCurrent(
  targets: string[],
  currentKey: string,
  outgoing: Map<string, string[]>,
) {
  const candidates = targets.filter((target) =>
    reaches(target, currentKey, outgoing),
  )
  return candidates.length === 1 ? candidates[0] : undefined
}

function reaches(
  start: string,
  target: string,
  outgoing: Map<string, string[]>,
) {
  const pending = [start]
  const visited = new Set<string>()
  while (pending.length) {
    const current = pending.pop()
    if (!current || visited.has(current)) continue
    if (current === target) return true
    visited.add(current)
    pending.push(...(outgoing.get(current) ?? []))
  }
  return false
}

function stateStage(state: ReleaseWorkflowState): WorkflowProgressStage {
  return {
    key: state.key,
    title: state.name,
    content: state.type,
    stateKeys: [state.key],
  }
}

function terminalOutcomeStage(
  states: ReleaseWorkflowState[],
  currentKey: string,
  copy: ProgressCopy,
): WorkflowProgressStage {
  const selected = states.find((state) => state.key === currentKey)
  return selected
    ? stateStage(selected)
    : {
        key: `terminal-result:${states.map((state) => state.key).join(':')}`,
        title: copy.resultTitle,
        content: copy.resultPending,
        stateKeys: states.map((state) => state.key),
      }
}
