export interface ModeMeta {
  id: string;
  name: string;
  tagline: string;
  roles: string[];
  toolCount: number;
  description: string;
}

export interface RoleMeta {
  code: string;
  name: string;
  description: string;
  prompt: string;
  tools: string[];
  modes: string[];
  model?: string;
}

export interface MainPromptMeta {
  id: string;
  name: string;
  tagline: string;
  prompt: string;
  tools: string[];
  roles: string[];
  model?: string;
}

export interface KnowledgeMeta {
  name: string;
  description: string;
  updatedAt: string;
}

export interface KnowledgeDoc extends KnowledgeMeta {
  body: string;
}

export interface Run {
  id: string;
  projectId: string;
  title: string;
  mode: string;
  target: string;
  objective: string;
  authorization: string;
  scope: string;
  status: string;
  providerId: string;
  model: string;
  goalMode?: boolean;
  createdAt: string;
  updatedAt: string;
  autoTurns: number;
  lastUserAt: string;
}

export interface RunSummary extends Run {
  goals: number;
  openGoals: number;
  intents: number;
  facts: number;
  findings: number;
  running: boolean;
}

export interface Goal {
  id: string;
  runId: string;
  text: string;
  acceptance: string;
  status: string;
  evidenceIds: string[];
  createdAt: string;
  updatedAt: string;
}

export interface Constraint {
  id: string;
  runId: string;
  kind: string;
  text: string;
  origin: string;
  active: boolean;
  createdAt: string;
}

export interface ToolCall {
  id: string;
  name: string;
  arguments: unknown;
}

export interface Message {
  id: string;
  sessionId: string;
  runId: string;
  role: "user" | "assistant" | "tool" | string;
  content: string;
  reasoning?: string;
  toolCalls?: ToolCall[] | null;
  toolCallId?: string;
  name?: string;
  visible: boolean;
  createdAt: string;
}

export interface GraphNode {
  id: string;
  runId: string;
  kind: string;
  payload: Record<string, any>;
  state: string;
  priority: number;
  origin: string;
  owner: string;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
}

export interface Edge {
  id: string;
  runId: string;
  srcId: string;
  dstId: string;
  rel: string;
  createdAt: string;
}

export interface Anchor {
  nodeId: string;
  assetId: string;
}

export interface Asset {
  id: string;
  type: string;
  value: string;
  identifier: string;
  parentId?: string;
  company: string;
  meta: string;
  firstSeenRun: string;
  lastSeen: string;
  createdAt: string;
}

export interface AssetCoverage extends Asset {
  tested: boolean;
}

export interface Coverage {
  total: number;
  tested: number;
  untested: Asset[];
  assets: AssetCoverage[];
  [key: string]: unknown;
}

export interface Evidence {
  id: string;
  runId: string;
  kind: string;
  label: string;
  sha256: string;
  size: number;
  channel: string;
  layers: string[];
  refPath: string;
  createdAt: string;
  preview?: string;
}

export interface Flow {
  id: string;
  runId: string;
  source: string;
  transform: string;
  sink: string;
  confidence: number;
  evidenceIds: string[];
  createdAt: string;
}

export interface Retest {
  id: string;
  runId: string;
  findingId: string;
  status: string;
  verdict: string;
  notes: string;
  evidenceIds: string[];
  createdAt: string;
  finishedAt?: string;
}

export interface Task {
  id: string;
  runId: string;
  parentId: string;
  nodeId: string;
  role: string;
  title: string;
  status: string;
  budget: number;
  inbox: { text: string; at: string }[];
  result: string;
  error: string;
  createdAt: string;
  startedAt?: string;
  finishedAt?: string;
}

export interface TaskDetail {
  task: Task;
  run: Run;
  project: Project;
  events: Event[];
}

export interface Event {
  id: string;
  runId: string;
  taskId: string;
  kind: string;
  tool: string;
  summary: string;
  detail: string;
  createdAt: string;
}

export interface Approval {
  id: string;
  runId: string;
  taskId: string;
  tool: string;
  input: string;
  reason: string;
  status: string;
  createdAt: string;
  decidedAt?: string;
}

export interface AuditEntry {
  id: string;
  runId: string;
  taskId: string;
  tool: string;
  action: string;
  reason: string;
  command: string;
  createdAt: string;
}

export interface RunDetail {
  run: Run;
  goals: Goal[];
  constraints: Constraint[];
  counts: Record<string, number>;
  coverage: Coverage;
  running: boolean;
  mode: { id: string; name: string; tagline: string; roles: string[] };
}

export interface ReviewItem {
  kind: string;
  ref: string;
  status: string;
  text: string;
}

export interface Snapshot {
  run: Run;
  goals: Goal[];
  constraints: Constraint[];
  nodes: GraphNode[];
  edges: Edge[];
  anchors: Anchor[];
  assets: Asset[];
  evidence: Evidence[];
  flows: Flow[];
  retests: Retest[];
  events: Event[];
  tasks: Task[];
}

export interface SnapshotView {
  snapshot: Snapshot;
  coverage: Coverage;
  review: { items: ReviewItem[] };
  running: boolean;
}

export interface FindingPayload {
  vulnclass?: string;
  title: string;
  severity: string;
  summary?: string;
  description?: string;
  impact?: string;
  trustBoundary?: string;
  reproducibleSteps: string[];
  evidenceIds?: string[];
  assetIds?: string[];
  reportMarkdown?: string;
}

export interface FindingView {
  id: string;
  state: string;
  createdAt: string;
  finding: FindingPayload;
  evidence: Evidence[];
}

export interface ProviderConfig {
  id: string;
  kind: string;
  baseUrl: string;
  apiKey: string;
  model: string;
  maxTokens?: number;
  maxTokensField?: string;
  contextWindow?: number;
  reasoningMode?: string;
  reasoningEffort?: string;
  reasoningProfile?: string;
  temperature?: number;
}

export interface MCPServerConfig {
  id: string;
  transport: string;
  command?: string;
  args?: string[];
  env?: Record<string, string>;
  url?: string;
  headers?: Record<string, string>;
  enabled: boolean;
}

export interface MCPStatus {
  id: string;
  transport: string;
  state: string;
  error?: string;
  tools: number;
  toolNames?: string[];
}

export interface SkillInfo {
  name: string;
  description: string;
  path: string;
  body?: string;
}

export interface AppConfig {
  listen: string;
  dataDir: string;
  workspace: string;
  skills: string[];
  providers: ProviderConfig[];
  activeProvider: string;
  mcp: MCPServerConfig[];
  approval: { mode: string; askTools?: string[] };
  auth: { enabled: boolean; username: string; sessionDays: number; passwordHash?: string; secret?: string };
  runtime: {
    maxIterations: number;
    maxOutputTokens: number;
    maxAutoTurns: number;
    contextMessages: number;
    contextWindow: number;
    toolResultChars: number;
    autoContinue: boolean;
    autoContinueWaitSeconds: number;
  };
}

export interface Project {
  id: string;
  name: string;
  nkey: string;
  description: string;
  authorization: string;
  scope: string;
  status: string;
  createdAt: string;
  updatedAt: string;
}

export interface ProjectSummary extends Project {
  conversations: number;
  findings: number;
  openGoals: number;
  assets: number;
  lastActivity: string;
}

export interface ProjectNode {
  id: string;
  projectId: string;
  conversationId: string;
  kind: string;
  payload: Record<string, any>;
  state: string;
  priority: number;
  pinned: boolean;
  visibility: string;
  origin: string;
  owner: string;
  sourceNodeId: string;
  createdAt: string;
  updatedAt: string;
}

export interface ProjectEdge {
  id: string;
  projectId: string;
  srcId: string;
  dstId: string;
  rel: string;
  createdAt: string;
}

export interface ProjectMemory {
  id: string;
  projectId: string;
  key: string;
  value: string;
  kind: string;
  sourceNodeId: string;
  updatedBy: string;
  updatedAt: string;
}

export interface ProjectFinding {
  id: string;
  projectId: string;
  projectName: string;
  conversationId: string;
  conversationTitle: string;
  payload: FindingPayload;
  state: string;
  createdAt: string;
}

export interface ProjectDetail {
  project: Project;
  conversations: Run[];
  assets: Asset[];
  board: ProjectNode[];
  edges: ProjectEdge[];
  memory: ProjectMemory[];
  findings: ProjectFinding[];
  coverage: Coverage;
}

export interface ProjectResponse {
  project: ProjectDetail;
  running: Record<string, boolean>;
}

export interface ToolCount {
  tool: string;
  count: number;
}

export interface ProjectFact {
  id: string;
  projectId: string;
  projectName: string;
  conversationId: string;
  conversationTitle: string;
  payload: Record<string, any>;
  state: string;
  createdAt: string;
}

export interface Capabilities {
  mcpTools: number;
  mcpOnline: number;
  skills: number;
  agents: number;
  modes: number;
}

export interface Dashboard {
  projects: number;
  conversations: number;
  running: number;
  findings: number;
  openGoals: number;
  assets: number;
  toolCalls: number;
  pendingApprovals: number;
  riskScore: number;
  severity: Record<string, number>;
  statusCounts: Record<string, number>;
  toolCounts: ToolCount[];
  recentFindings: ProjectFinding[];
  recentFacts: ProjectFact[];
  recentEvents: Event[];
  capabilities: Capabilities;
}

export interface AuthStatus {
  enabled: boolean;
  authenticated: boolean;
  username: string;
}
