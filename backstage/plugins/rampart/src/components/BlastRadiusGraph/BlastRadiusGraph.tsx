import { useMemo, useState } from 'react';
import dagre from 'dagre';
import ReactFlow, {
  Background,
  Controls,
  MiniMap,
  type Edge,
  type Node,
  type NodeMouseHandler,
} from 'reactflow';
import 'reactflow/dist/style.css';

import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

import type { components } from '../../api/gen/schema';

type IncidentDetail = components['schemas']['IncidentDetail'];
type Component = components['schemas']['Component'];

export type BlastRadiusGraphProps = {
  detail: IncidentDetail;
  /**
   * Optional component ref to focus on. When set, the graph re-roots
   * around this component (highlighted as the centre node). Double-
   * clicking any other node updates this via `onRootChange`.
   */
  rootComponentRef?: string;
  /** Called when the user double-clicks a component node to re-root. */
  onRootChange?: (componentRef: string) => void;
};

/** GRAPH_HEIGHT_PX matches the modal body height in IncidentDetailDrawer
 *  so reactflow has explicit canvas dimensions — without this it
 *  collapses to 0×0 and renders nothing. */
const GRAPH_HEIGHT_PX = 480;

/** AFFECTATION colours map the three categories called out in
 *  ADR-0011 Theme E2. Today the engine only surfaces "directly
 *  affected" (incident.affectedComponentsSnapshot is the leaf set);
 *  transitive vs. safe categories are placeholders pending the
 *  catalog-graph enrichment in v0.3.0. The colour map is exhaustive
 *  so the future enrichment is a one-line dispatch change. */
const AFFECTATION_COLOUR = {
  incident: '#1976d2', // primary blue — the centre node
  directly: '#d32f2f', // red — directly affected component
  transitively: '#ed6c02', // orange — depends on a directly-affected
  safe: '#9e9e9e', // grey — in the catalog but not affected
} as const;

type Affectation = keyof typeof AFFECTATION_COLOUR;

type GraphNodeData = {
  label: string;
  affectation: Affectation;
  componentRef?: string;
};

/**
 * BlastRadiusGraph renders the affected component set as a star graph:
 * the incident sits in the centre and every affected component hangs
 * off it. The graph layout is dagre's left-to-right tree which gives
 * the centre→leaves visual without overlapping nodes for typical
 * fixture sizes (≤ 20 affected components).
 *
 * Pan/zoom + minimap come from reactflow's defaults. Single-click
 * highlights a node (keeps it selected); double-click on a component
 * node re-roots the graph around that component via `onRootChange`.
 *
 * Catalog-graph enrichment (depends_on edges + transitive affectation
 * + safe-component overlay) is deferred to v0.3.0 once the engine
 * surfaces the dependency structure. Today the graph is incident-
 * centric — this is enough to demo "which services blew up" without
 * pulling Backstage catalog state into the plugin.
 */
export const BlastRadiusGraph = ({
  detail,
  rootComponentRef,
  onRootChange,
}: BlastRadiusGraphProps) => {
  const [selectedRef, setSelectedRef] = useState<string | null>(null);

  const { nodes, edges } = useMemo(
    () => buildGraph(detail, rootComponentRef ?? null),
    [detail, rootComponentRef],
  );

  if (nodes.length <= 1) {
    return (
      <Box sx={{ p: 2 }}>
        <Typography variant="body2" color="text.secondary">
          No affected components to graph.
        </Typography>
      </Box>
    );
  }

  const handleNodeClick: NodeMouseHandler = (_event, node) => {
    const data = node.data as GraphNodeData;
    if (data.componentRef) {
      setSelectedRef(data.componentRef);
    }
  };

  const handleNodeDoubleClick: NodeMouseHandler = (_event, node) => {
    const data = node.data as GraphNodeData;
    if (data.componentRef && onRootChange) {
      onRootChange(data.componentRef);
    }
  };

  return (
    <Box
      data-testid="blast-radius-graph"
      sx={{ width: '100%', height: GRAPH_HEIGHT_PX, border: '1px solid #ddd' }}
    >
      <ReactFlow
        nodes={nodes.map(n => ({
          ...n,
          style: nodeStyleFor(n.data as GraphNodeData, selectedRef),
        }))}
        edges={edges}
        onNodeClick={handleNodeClick}
        onNodeDoubleClick={handleNodeDoubleClick}
        fitView
        proOptions={{ hideAttribution: true }}
      >
        <Background />
        <Controls />
        <MiniMap zoomable pannable />
      </ReactFlow>
    </Box>
  );
};

function nodeStyleFor(data: GraphNodeData, selectedRef: string | null) {
  const colour = AFFECTATION_COLOUR[data.affectation];
  const isSelected = data.componentRef !== undefined && data.componentRef === selectedRef;
  return {
    border: `2px solid ${colour}`,
    borderRadius: 8,
    padding: 8,
    background: '#fff',
    boxShadow: isSelected ? `0 0 0 3px ${colour}40` : 'none',
    fontSize: 12,
    width: 200,
  };
}

/** buildGraph wires the dagre auto-layout: the incident is the root,
 *  every affected component is a leaf, and edges are incident→leaf. */
function buildGraph(detail: IncidentDetail, root: string | null) {
  const components = detail.affectedComponents ?? [];
  const incidentLabel = `Incident ${detail.incident.id.slice(-8)}`;

  const incidentNode: Node<GraphNodeData> = {
    id: 'incident',
    data: {
      label: incidentLabel,
      affectation: 'incident',
    },
    position: { x: 0, y: 0 },
  };

  const componentNodes: Node<GraphNodeData>[] = components.map(c => ({
    id: c.ref,
    data: {
      label: componentLabel(c),
      // Every entry in affectedComponentsSnapshot is "directly affected"
      // by definition. The transitive / safe categories will surface
      // when the catalog-graph enrichment lands (v0.3.0).
      affectation: c.ref === root ? 'transitively' : 'directly',
      componentRef: c.ref,
    },
    position: { x: 0, y: 0 },
  }));

  const edges: Edge[] = components.map(c => ({
    id: `incident-${c.ref}`,
    source: 'incident',
    target: c.ref,
    animated: true,
  }));

  const laidOut = layoutWithDagre([incidentNode, ...componentNodes], edges);
  return { nodes: laidOut, edges };
}

function componentLabel(c: Component): string {
  const owner = c.owner ?? 'unknown';
  return `${c.name ?? c.ref}\n${c.namespace ?? ''}\nowner: ${owner}`;
}

/** layoutWithDagre runs dagre's left-to-right tree layout over the
 *  reactflow node + edge set. Returns the same nodes with `position`
 *  populated. Node dimensions are fixed (matches `nodeStyleFor`'s
 *  width above). */
function layoutWithDagre(nodes: Node<GraphNodeData>[], edges: Edge[]): Node<GraphNodeData>[] {
  const g = new dagre.graphlib.Graph();
  g.setGraph({ rankdir: 'LR', nodesep: 24, ranksep: 64 });
  g.setDefaultEdgeLabel(() => ({}));

  const nodeWidth = 220;
  const nodeHeight = 80;
  for (const n of nodes) {
    g.setNode(n.id, { width: nodeWidth, height: nodeHeight });
  }
  for (const e of edges) {
    g.setEdge(e.source, e.target);
  }
  dagre.layout(g);

  return nodes.map(n => {
    const pos = g.node(n.id);
    return {
      ...n,
      position: { x: pos.x - nodeWidth / 2, y: pos.y - nodeHeight / 2 },
    };
  });
}
