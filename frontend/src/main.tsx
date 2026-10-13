import {createRoot} from 'react-dom/client';
import './style.css';
import App from './app/App';
const theme = window.matchMedia('(prefers-color-scheme: dark)');
const syncTheme = () => {document.documentElement.classList.toggle('dark', theme.matches);document.documentElement.dataset.theme = theme.matches ? 'dark' : 'light'};
syncTheme(); theme.addEventListener('change', syncTheme);
createRoot(document.getElementById('root')!).render(<App />);
