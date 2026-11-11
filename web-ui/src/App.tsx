import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Header from './components/Header';
import { RefreshProvider, ThemeProvider } from './context';
import Dashboard from './pages/Dashboard';
import VPCs from './pages/VPCs';
import Subnets from './pages/Subnets';
import Gateways from './pages/Gateways';
import Services from './pages/Services';
import NFV from './pages/NFV';
import Infra from './pages/Infra';
import Topology from './pages/Topology';
import Metrics from './pages/Metrics';
import EntityDetail from './pages/EntityDetail';

export default function App() {
  return (
    <ThemeProvider>
      <RefreshProvider>
        <BrowserRouter>
          <div className="flex h-screen bg-gray-50 text-gray-900 dark:bg-gray-950 dark:text-gray-100">
            <Sidebar />
            <div className="flex min-w-0 flex-1 flex-col">
              <Header />
              <main className="flex-1 overflow-y-auto p-6 lg:p-8">
                <Routes>
                  <Route path="/" element={<Dashboard />} />
                  <Route path="/vpcs" element={<VPCs />} />
                  <Route path="/subnets" element={<Subnets />} />
                  <Route path="/gateways" element={<Gateways />} />
                  <Route path="/services" element={<Services />} />
                  <Route path="/nfv" element={<NFV />} />
                  <Route path="/infra" element={<Infra />} />
                  <Route path="/topology" element={<Topology />} />
                  <Route path="/metrics" element={<Metrics />} />
                  <Route path="/:type/:ns/:name" element={<EntityDetail />} />
                </Routes>
              </main>
            </div>
          </div>
        </BrowserRouter>
      </RefreshProvider>
    </ThemeProvider>
  );
}
