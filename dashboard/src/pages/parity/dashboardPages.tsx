import { Navigate } from 'react-router-dom';
import { useParityQuery } from '../../api/parity/withFallback';
import { parityApi } from '../../api/parity/parityApi';
import * as mocks from '../../api/parity/mocks';
import { GenericParityView } from '../../components/parity/GenericParityView';
import { ParityMockBanner } from '../../components/parity/ParityMockBanner';
import StatCard from '../../components/StatCard';
import { Activity, Server, Shield } from 'lucide-react';

export function DashboardServicePage() {
    const q = useParityQuery(['parity', 'dashboard', 'service-summary'], () => parityApi.getServiceSummary(), mocks.mockServiceSummary);

    if (q.isLoading) return <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />;
    if (q.isError || !q.data) return null;
    const { data: rawData, isMock } = q.data;
    const data = rawData as {
        sla_percent: number;
        events_ingested: number;
        avg_detection_latency_ms: number;
        pipeline_health: string;
        [key: string]: unknown;
    };

    return (
        <div className="space-y-4">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Service summary</h2>
            {isMock && <ParityMockBanner />}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="SLA" value={`${data.sla_percent}%`} icon={Activity} color="emerald" />
                <StatCard title="Events ingested" value={String(data.events_ingested)} icon={Server} />
                <StatCard title="Avg detection (ms)" value={String(data.avg_detection_latency_ms)} icon={Shield} />
                <StatCard title="Pipeline" value={data.pipeline_health} icon={Activity} color="cyan" />
            </div>
            <details className="rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-800/80 p-4">
                <summary className="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-300">Raw payload</summary>
                <pre className="text-xs mt-3 overflow-auto max-h-64 font-mono text-gray-600 dark:text-gray-400">
                    {JSON.stringify(data, null, 2)}
                </pre>
            </details>
        </div>
    );
}

export function DashboardEndpointPage() {
    const q = useParityQuery(['parity', 'dashboard', 'endpoint-summary'], () => parityApi.getEndpointSummary(), mocks.mockEndpointSummary);

    if (q.isLoading) return <div className="h-40 rounded-xl bg-gray-100 dark:bg-gray-800 animate-pulse" />;
    if (q.isError || !q.data) return null;
    const { data: rawData, isMock } = q.data;
    const data = rawData as {
        totals: { endpoints_total: number; online: number; offline: number; isolated: number };
        risk_distribution: unknown;
        [key: string]: unknown;
    };

    return (
        <div className="space-y-4">
            <h2 className="text-lg font-semibold text-gray-900 dark:text-white">Endpoint summary</h2>
            {isMock && <ParityMockBanner />}
            <div className="grid grid-cols-2 lg:grid-cols-4 gap-4">
                <StatCard title="Endpoints" value={String(data.totals.endpoints_total)} icon={Server} />
                <StatCard title="Online" value={String(data.totals.online)} icon={Activity} color="emerald" />
                <StatCard title="Offline" value={String(data.totals.offline)} icon={Server} />
                <StatCard title="Isolated" value={String(data.totals.isolated)} icon={Shield} color="amber" />
            </div>
            <div className="rounded-xl border border-gray-200 dark:border-gray-700 p-4 bg-white dark:bg-gray-800/80">
                <h3 className="text-sm font-semibold text-gray-700 dark:text-gray-300 mb-2">Risk distribution</h3>
                <pre className="text-xs font-mono text-gray-600 dark:text-gray-400 overflow-auto">
                    {JSON.stringify(data.risk_distribution, null, 2)}
                </pre>
            </div>
        </div>
    );
}

export function DashboardCloudPage() {
    return (
        <GenericParityView
            title="Cloud summary"
            description="Cloud posture and findings overview."
            queryKey={['parity', 'dashboard', 'cloud']}
            fetcher={() => parityApi.getCloudSummary()}
            mock={mocks.mockCloudSummary}
        />
    );
}

export function DashboardAuditRedirect() {
    return <Navigate to="/audit" replace />;
}

export function DashboardEndpointCompliancePage() {
    return (
        <GenericParityView
            title="Endpoint compliance"
            queryKey={['parity', 'compliance', 'endpoint']}
            fetcher={() => parityApi.getComplianceEndpoint()}
            mock={mocks.mockComplianceEndpointRows}
        />
    );
}

export function DashboardCtemPage() {
    return (
        <div className="space-y-6">
            <GenericParityView
                title="CTEM exposure summary"
                queryKey={['parity', 'ctem', 'exposure']}
                fetcher={() => parityApi.getCtemExposureSummary()}
                mock={mocks.mockCtemExposure}
            />
            <GenericParityView
                title="CTEM findings"
                queryKey={['parity', 'ctem', 'findings']}
                fetcher={() => parityApi.getCtemFindings()}
                mock={mocks.mockCtemFindings.data}
            />
        </div>
    );
}

export function DashboardVerdictCloudPage() {
    return (
        <GenericParityView
            title="Verdict Cloud"
            queryKey={['parity', 'dashboard', 'verdict']}
            fetcher={() => parityApi.getVerdictCloud()}
            mock={mocks.mockVerdictCloud}
        />
    );
}

export function DashboardRoiPage() {
    return (
        <GenericParityView
            title="ROI dashboard"
            queryKey={['parity', 'dashboard', 'roi']}
            fetcher={() => parityApi.getRoi()}
            mock={mocks.mockRoi}
        />
    );
}

export function DashboardReportsPage() {
    return (
        <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-600 p-8 text-center text-gray-500 dark:text-gray-400">
            <p className="font-medium text-gray-700 dark:text-gray-300">Reports</p>
            <p className="text-sm mt-2">Scheduled PDF/CSV reports will connect here when the API is available.</p>
        </div>
    );
}

export function DashboardNotificationsPage() {
    return (
        <div className="rounded-xl border border-dashed border-gray-300 dark:border-gray-600 p-8 text-center text-gray-500 dark:text-gray-400">
            <p className="font-medium text-gray-700 dark:text-gray-300">Notifications</p>
            <p className="text-sm mt-2">In-app notification center — wiring to `/api/v1/...` when ready.</p>
        </div>
    );
}
