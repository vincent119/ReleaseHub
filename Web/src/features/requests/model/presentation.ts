export function requestStatusColor(status: string) {
  if (status === 'Succeeded') return 'success'
  if (status === 'Failed' || status === 'PartialFailed') return 'error'
  if (status === 'Blocked' || status === 'Terminated') return 'warning'
  if (status === 'Deploying' || status === 'Approved') return 'processing'
  if (status === 'Superseded') return 'default'
  return 'blue'
}

export function shortRequestID(id: string) {
  return id.length > 20 ? `${id.slice(0, 8)}…${id.slice(-8)}` : id
}

export function requestStatusLabel(status: string) {
  return status === 'PartialFailed' ? 'Partial Failed' : status
}

export function nodeStatusColor(status: string) {
  if (status === 'Succeeded') return 'success'
  if (status === 'Failed') return 'error'
  if (status === 'Blocked' || status === 'Terminated') return 'warning'
  if (status === 'Syncing' || status === 'Stabilizing') return 'processing'
  return 'default'
}
