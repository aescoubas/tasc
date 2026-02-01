import React, { useMemo, useState } from 'react';
import useSWR, { mutate } from 'swr';
import { Row, Col, Card, Spinner, Alert } from 'react-bootstrap';
import { DndContext, DragEndEvent, DragOverlay, DragStartEvent, useDraggable, useDroppable } from '@dnd-kit/core';
import { Task } from '../types';
import { fetchTasks, updateTask } from '../api';

// Draggable Task Component
const DraggableTask = ({ task }: { task: Task }) => {
  const { attributes, listeners, setNodeRef, transform } = useDraggable({
    id: task.id,
    data: { task },
  });

  const style = transform ? {
    transform: `translate3d(${transform.x}px, ${transform.y}px, 0)`,
    marginBottom: '8px',
    cursor: 'grab',
    opacity: 0.8,
  } : {
    marginBottom: '8px',
    cursor: 'grab',
  };

  return (
    <div ref={setNodeRef} style={style} {...listeners} {...attributes}>
      <Card className="shadow-sm">
        <Card.Body className="p-2">
           <div className="d-flex justify-content-between">
              <span className="fw-bold" style={{fontSize: '0.8rem', color: '#6c757d'}}>{task.project}</span>
              <span style={{fontSize: '0.8rem', color: '#adb5bd'}}>#{task.id}</span>
           </div>
           <div className="mt-1">{task.description}</div>
        </Card.Body>
      </Card>
    </div>
  );
};

// Droppable Column Component
const KanbanColumn = ({ id, title, tasks }: { id: string, title: string, tasks: Task[] }) => {
  const { setNodeRef } = useDroppable({
    id: id,
  });

  return (
    <Col md={3} className="d-flex flex-column h-100">
      <div className="bg-light rounded p-3 h-100 d-flex flex-column" style={{minHeight: '500px'}}>
        <h6 className="text-uppercase fw-bold text-secondary mb-3">{title} <span className="badge bg-secondary rounded-pill ms-1">{tasks.length}</span></h6>
        <div ref={setNodeRef} className="flex-grow-1">
          {tasks.map(t => (
            <DraggableTask key={t.id} task={t} />
          ))}
        </div>
      </div>
    </Col>
  );
};

const KanbanBoard: React.FC = () => {
  const { data: tasks, error, isLoading } = useSWR<Task[]>('tasks', fetchTasks);
  const [activeTask, setActiveTask] = useState<Task | null>(null);

  // Local state for optimistic UI updates could be added here, 
  // but for simplicity we'll rely on SWR revalidation or fast local mutation.
  
  const columns = useMemo(() => {
    if (!tasks) return { backlog: [], ongoing: [], blocked: [], done: [] };
    return {
      backlog: tasks.filter(t => t.status === 'backlog' || t.status === 'undefined'),
      ongoing: tasks.filter(t => t.status === 'ongoing'),
      blocked: tasks.filter(t => t.status === 'blocked' || t.is_blocked), // Note: is_blocked might override status visual
      done: tasks.filter(t => t.status === 'done'),
    };
  }, [tasks]);

  const handleDragStart = (event: DragStartEvent) => {
    if (event.active.data.current) {
        setActiveTask(event.active.data.current.task);
    }
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    setActiveTask(null);

    if (!over) return;

    const taskId = active.id as number;
    const newStatus = over.id as string;
    
    // Find task
    const task = tasks?.find(t => t.id === taskId);
    if (!task) return;

    if (task.status === newStatus) return;

    // Optimistic Update
    const updatedTask = { ...task, status: newStatus as any };
    const updatedTasks = tasks?.map(t => t.id === taskId ? updatedTask : t);
    
    mutate('tasks', updatedTasks, false); // Update cache locally

    try {
        await updateTask(updatedTask);
        mutate('tasks'); // Revalidate to be sure
    } catch (e) {
        console.error("Failed to update task", e);
        mutate('tasks'); // Revert on error
    }
  };

  if (isLoading) return <div className="text-center p-5"><Spinner animation="border" /></div>;
  if (error) return <Alert variant="danger">Error loading tasks</Alert>;

  return (
    <DndContext onDragStart={handleDragStart} onDragEnd={handleDragEnd}>
      <Row className="g-4">
        <KanbanColumn id="backlog" title="Backlog" tasks={columns.backlog} />
        <KanbanColumn id="ongoing" title="In Progress" tasks={columns.ongoing} />
        <KanbanColumn id="blocked" title="Blocked" tasks={columns.blocked} />
        <KanbanColumn id="done" title="Done" tasks={columns.done} />
      </Row>
      
      <DragOverlay>
         {activeTask ? (
             <Card className="shadow" style={{width: '250px'}}>
                 <Card.Body className="p-2">
                     <div className="d-flex justify-content-between">
                        <span className="fw-bold" style={{fontSize: '0.8rem', color: '#6c757d'}}>{activeTask.project}</span>
                     </div>
                     <div className="mt-1">{activeTask.description}</div>
                 </Card.Body>
             </Card>
         ) : null}
      </DragOverlay>
    </DndContext>
  );
};

export default KanbanBoard;
