import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Sidebar from './components/Sidebar';
import Dashboard from './pages/Dashboard';
import VPCs from './pages/VPCs';
import Subnets from './pages/Subnets';
import Gateways from './pages/Gateways';
import Services from './pages/Services';
import NFV from './pages/NFV';
import Infra from './pages/Infra';
import Topology from './pages/Topology';

export default function App() {
  return (
    <BrowserRouter>
      <div className="flex h-screen bg-gray-950 text-gray-100">
        <Sidebar />
        <main className="flex-1 overflow-y-auto p-8">
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/vpcs" element={<VPCs />} />
            <Route path="/subnets" element={<Subnets />} />
            <Route path="/gateways" element={<Gateways />} />
            <Route path="/services" element={<Services />} />
            <Route path="/nfv" element={<NFV />} />
            <Route path="/infra" element={<Infra />} />
            <Route path="/topology" element={<Topology />} />
          </Routes>
        </main>
      </div>
    </BrowserRouter>
  );
}
