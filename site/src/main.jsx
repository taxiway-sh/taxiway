import { ViteReactSSG } from 'vite-react-ssg';
import { routes } from './docs/routes.jsx';
import './styles/styles.css';

export const createRoot = ViteReactSSG({ routes, basename: import.meta.env.BASE_URL });
