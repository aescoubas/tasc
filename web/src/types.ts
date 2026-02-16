export interface Task {
  id: number;
  description: string;
  project: string;
  status: 'backlog' | 'ongoing' | 'done' | 'blocked' | 'deleted' | 'undefined';
  created_at: string;
  due_at?: string;
  estimate?: string;
  is_blocked?: boolean;
}

export interface Project {
  name: string;
  description: string;
  status: 'active' | 'archived';
}
