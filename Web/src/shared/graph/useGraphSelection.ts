import { useState } from 'react'

export function useGraphSelection() {
  const [selectedNodeID, setSelectedNodeID] = useState<string>()
  const [selectedEdgeID, setSelectedEdgeID] = useState<string>()

  const selectNode = (id: string) => {
    setSelectedNodeID(id)
    setSelectedEdgeID(undefined)
  }
  const selectEdge = (id: string) => {
    setSelectedEdgeID(id)
    setSelectedNodeID(undefined)
  }
  const clearSelection = () => {
    setSelectedNodeID(undefined)
    setSelectedEdgeID(undefined)
  }

  return {
    selectedNodeID,
    selectedEdgeID,
    selectNode,
    selectEdge,
    clearSelection,
  }
}
