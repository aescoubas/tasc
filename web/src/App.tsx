import { useState } from 'react';
import { Container, Navbar, Nav, Tab, Tabs } from 'react-bootstrap';
import TaskList from './components/TaskList';
import KanbanBoard from './components/KanbanBoard';

function App() {
  const [key, setKey] = useState('kanban');

  return (
    <>
      <Navbar bg="dark" variant="dark" expand="lg" className="mb-4">
        <Container>
          <Navbar.Brand href="#home">Tasc Dashboard</Navbar.Brand>
          <Navbar.Toggle aria-controls="basic-navbar-nav" />
          <Navbar.Collapse id="basic-navbar-nav">
            <Nav className="me-auto">
              <Nav.Link onClick={() => setKey('kanban')}>Board</Nav.Link>
              <Nav.Link onClick={() => setKey('list')}>List</Nav.Link>
            </Nav>
          </Navbar.Collapse>
        </Container>
      </Navbar>

      <Container>
        <Tabs
          id="view-tabs"
          activeKey={key}
          onSelect={(k) => setKey(k || 'kanban')}
          className="mb-3"
        >
          <Tab eventKey="kanban" title="Kanban Board">
             <KanbanBoard />
          </Tab>
          <Tab eventKey="list" title="Task List">
             <TaskList />
          </Tab>
        </Tabs>
      </Container>
    </>
  );
}

export default App;
