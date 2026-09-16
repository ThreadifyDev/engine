import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router';
import App from './root';
import './styles/tailwind.css';

createRoot(document.getElementById('root')!).render(<BrowserRouter><App /></BrowserRouter>);
