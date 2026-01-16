export interface AppNode {
  id: string;
  parent_id?: string;
  title: string;
  content?: string;
  columns?: Column[];
  children?: AppNode[];
  created: string;
  modified: string;
  type: 'document' | 'database' | 'hybrid';
}

export interface Column {
  id: string;
  name: string;
  type: string;
  options?: string[];
  required?: boolean;
}

export interface DataRecord {
  id: string;
  data: Record<string, unknown>;
  created: string;
  modified: string;
}

export interface Commit {
  hash: string;
  message: string;
  timestamp: string;
}

export interface User {
  id: string;
  email: string;
  name: string;
  organization_id: string;
  role: 'admin' | 'editor' | 'viewer';
}
