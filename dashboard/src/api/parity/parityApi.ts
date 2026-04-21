/**
 * Platform API calls (same base as Sigma Engine: `/api/v1`).
 * Consumers should wrap with `withParityFallback` or `useParityQuery` for silent mock on 404/network.
 */
import { sigmaApi } from '../client';
import type { AxiosResponse } from 'axios';

function unwrap<T>(res: AxiosResponse<{ data?: T } | T>): T {
    const body = res.data as { data?: T };
    if (body && typeof body === 'object' && 'data' in body && body.data !== undefined) {
        return body.data as T;
    }
    return res.data as T;
}

async function get<T>(url: string, params?: Record<string, string>): Promise<T> {
    const res = await sigmaApi.get<{ data?: T } | T>(url, { params });
    return unwrap(res);
}

async function post<T>(url: string, body?: Record<string, unknown>): Promise<T> {
    const res = await sigmaApi.post<{ data?: T } | T>(url, body);
    return unwrap(res);
}

async function patch<T>(url: string, body?: Record<string, unknown>): Promise<T> {
    const res = await sigmaApi.patch<{ data?: T } | T>(url, body);
    return unwrap(res);
}

export const parityApi = {
    getServiceSummary: (params?: Record<string, string>) =>
        get('/api/v1/dashboard/service-summary', params),

    getEndpointSummary: (params?: Record<string, string>) =>
        get('/api/v1/dashboard/endpoint-summary', params),

    getCloudSummary: (params?: Record<string, string>) =>
        get('/api/v1/dashboard/cloud-summary', params),

    getRoi: (params?: Record<string, string>) =>
        get('/api/v1/dashboard/roi', params),

    getVerdictCloud: (params?: Record<string, string>) =>
        get('/api/v1/dashboard/verdict-cloud', params),

    getComplianceEndpoint: (params?: Record<string, string>) =>
        get('/api/v1/compliance/endpoint', params),

    getComplianceControlDevices: (controlId: string, params?: Record<string, string>) =>
        get(`/api/v1/compliance/controls/${encodeURIComponent(controlId)}/devices`, params),

    getCtemExposureSummary: (params?: Record<string, string>) =>
        get('/api/v1/ctem/exposure-summary', params),

    getCtemFindings: (params?: Record<string, string>) =>
        get('/api/v1/ctem/findings', params),

    patchCtemFindingStatus: (id: string, body: { status: string; comment?: string }) =>
        patch(`/api/v1/ctem/findings/${encodeURIComponent(id)}/status`, body as Record<string, unknown>),

    getVulnFindings: (params?: Record<string, string>) =>
        get('/api/v1/vuln/findings', params),

    getVulnAsset: (assetId: string) =>
        get(`/api/v1/vuln/assets/${encodeURIComponent(assetId)}`),

    postVulnRemediate: (body: Record<string, unknown>) =>
        post('/api/v1/vuln/tasks/remediate', body),

    getPatchOverview: (params?: Record<string, string>) =>
        get('/api/v1/patch/overview', params),

    getPatchMissing: (params?: Record<string, string>) =>
        get('/api/v1/patch/missing', params),

    postPatchDeploy: (body: Record<string, unknown>) =>
        post('/api/v1/patch/deploy', body),

    getSecurityPostureEndpoint: (params?: Record<string, string>) =>
        get('/api/v1/security/posture/endpoint', params),

    getSecurityPostureCloud: (params?: Record<string, string>) =>
        get('/api/v1/security/posture/cloud', params),

    getSiemConnectors: (params?: Record<string, string>) =>
        get('/api/v1/siem/connectors', params),

    postSiemConnector: (body: Record<string, unknown>) =>
        post('/api/v1/siem/connectors', body),

    getThreatLabsIocs: (params?: Record<string, string>) =>
        get('/api/v1/threat-labs/iocs', params),

    postThreatLabsIocsSearch: (body: Record<string, unknown>) =>
        post('/api/v1/threat-labs/iocs/search', body),

    getManagedIncidents: (params?: Record<string, string>) =>
        get('/api/v1/managed-security/incidents', params),

    getManagedSla: (params?: Record<string, string>) =>
        get('/api/v1/managed-security/sla', params),

    getItsmTickets: (params?: Record<string, string>) =>
        get('/api/v1/itsm/tickets', params),

    postItsmTicket: (body: Record<string, unknown>) =>
        post('/api/v1/itsm/tickets', body),

    getItsmPlaybooks: (params?: Record<string, string>) =>
        get('/api/v1/itsm/playbooks', params),

    postItsmAutomation: (body: Record<string, unknown>) =>
        post('/api/v1/itsm/automations', body),

    getManagementDevices: (params?: Record<string, string>) =>
        get('/api/v1/management/devices', params),

    getManagementProfiles: (params?: Record<string, string>) =>
        get('/api/v1/management/profiles', params),

    postManagementProfile: (body: Record<string, unknown>) =>
        post('/api/v1/management/profiles', body),

    getRmmJobs: (params?: Record<string, string>) =>
        get('/api/v1/management/rmm/jobs', params),

    getAppControlPolicies: (params?: Record<string, string>) =>
        get('/api/v1/management/application-control/policies', params),

    getLicenses: () => get('/api/v1/management/licenses'),

    getBillingSummary: (params?: Record<string, string>) =>
        get('/api/v1/management/billing/summary', params),
};

