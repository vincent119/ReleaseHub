export function requestStatusColor(status: string) {
  if (status === 'Succeeded') return 'success'
  if (status === 'Failed' || status === 'PartialFailed') return 'error'
  if (status === 'Blocked' || status === 'Terminated') return 'warning'
  if (status === 'Deploying' || status === 'Approved') return 'processing'
  if (status === 'Superseded') return 'default'
  return 'blue'
}

export function nodeStatusColor(status: string) {
  if (status === 'Succeeded') return 'success'
  if (status === 'Failed') return 'error'
  if (status === 'Blocked' || status === 'Terminated') return 'warning'
  if (status === 'Syncing' || status === 'Stabilizing') return 'processing'
  return 'default'
}
