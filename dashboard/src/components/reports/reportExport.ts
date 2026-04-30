/**
 * Report Export Functions
 * Export reports to various formats: PDF, Excel, Word, HTML, CSV, JSON
 */

import type { ReportData, ReportTemplate, ReportFormat } from './ReportTemplates';
import { REPORT_TEMPLATES } from './ReportTemplates';

// Export report to selected format
export async function exportReport(
    data: ReportData,
    format: ReportFormat,
    template: ReportTemplate
): Promise<void> {
    switch (format) {
        case 'pdf':
            await exportToPDF(data, template);
            break;
        case 'excel':
            await exportToExcel(data, template);
            break;
        case 'word':
            await exportToWord(data, template);
            break;
        case 'html':
            await exportToHTML(data, template);
            break;
        case 'csv':
            await exportToCSV(data, template);
            break;
        case 'json':
            await exportToJSON(data, template);
            break;
        default:
            throw new Error(`Unsupported format: ${format}`);
    }
}

// Export to PDF using browser print to PDF
async function exportToPDF(data: ReportData, template: ReportTemplate): Promise<void> {
    const config = REPORT_TEMPLATES[template];
    const html = generateHTMLReport(data, template, config.name);
    
    // Open in new window for print to PDF
    const printWindow = window.open('', '_blank');
    if (!printWindow) {
        throw new Error('Please allow popups to export PDF');
    }
    
    printWindow.document.write(html);
    printWindow.document.close();
    
    // Auto-trigger print after a delay to allow styles to load
    setTimeout(() => {
        printWindow.print();
    }, 500);
}

// Export to Excel (XLSX)
async function exportToExcel(data: ReportData, template: ReportTemplate): Promise<void> {
    // Dynamic import of xlsx library
    const XLSX = await import('xlsx');
    
    const workbook = XLSX.utils.book_new();
    
    // Summary sheet
    const summaryData = [
        ['Metric', 'Value'],
        ['Report Period', `${new Date(data.period.from).toLocaleDateString()} - ${new Date(data.period.to).toLocaleDateString()}`],
        ['Total Alerts', data.summary.totalAlerts],
        ['Critical', data.summary.criticalCount],
        ['High', data.summary.highCount],
        ['Medium', data.summary.mediumCount],
        ['Low', data.summary.lowCount],
        ['Total Devices', data.summary.totalDevices],
        ['Generated At', new Date(data.generatedAt).toLocaleString()],
    ];
    const summarySheet = XLSX.utils.aoa_to_sheet(summaryData);
    XLSX.utils.book_append_sheet(workbook, summarySheet, 'Summary');
    
    // Alerts sheet
    if (data.tables.alerts.length > 0) {
        const alertsSheet = XLSX.utils.json_to_sheet(data.tables.alerts);
        XLSX.utils.book_append_sheet(workbook, alertsSheet, 'Alerts');
    }
    
    // Devices sheet
    if (data.tables.devices.length > 0) {
        const devicesSheet = XLSX.utils.json_to_sheet(data.tables.devices);
        XLSX.utils.book_append_sheet(workbook, devicesSheet, 'Devices');
    }
    
    // Commands sheet
    if (data.tables.commands.length > 0) {
        const commandsSheet = XLSX.utils.json_to_sheet(data.tables.commands);
        XLSX.utils.book_append_sheet(workbook, commandsSheet, 'Commands');
    }
    
    // Timeline sheet
    if (data.charts.timeline.length > 0) {
        const timelineSheet = XLSX.utils.json_to_sheet(data.charts.timeline);
        XLSX.utils.book_append_sheet(workbook, timelineSheet, 'Timeline');
    }
    
    // Download
    const fileName = `EDR-Report-${template}-${new Date().toISOString().slice(0, 10)}.xlsx`;
    XLSX.writeFile(workbook, fileName);
}

// Export to Word (DOCX)
async function exportToWord(data: ReportData, template: ReportTemplate): Promise<void> {
    const config = REPORT_TEMPLATES[template];
    
    // Create HTML that works well when pasted into Word
    const html = generateHTMLReport(data, template, config.name, true);
    
    // Create a blob and download
    const blob = new Blob([html], { type: 'application/msword' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `EDR-Report-${template}-${new Date().toISOString().slice(0, 10)}.doc`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// Export to HTML
async function exportToHTML(data: ReportData, template: ReportTemplate): Promise<void> {
    const config = REPORT_TEMPLATES[template];
    const html = generateHTMLReport(data, template, config.name);
    
    const blob = new Blob([html], { type: 'text/html' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `EDR-Report-${template}-${new Date().toISOString().slice(0, 10)}.html`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// Export to CSV
async function exportToCSV(data: ReportData, template: ReportTemplate): Promise<void> {
    // Combine all data into CSV format
    const rows: string[][] = [];
    
    // Header
    rows.push(['EDR Security Report']);
    rows.push(['Period:', `${data.period.from} to ${data.period.to}`]);
    rows.push(['Generated:', data.generatedAt]);
    rows.push([]);
    
    // Summary
    rows.push(['SUMMARY']);
    rows.push(['Metric', 'Value']);
    rows.push(['Total Alerts', String(data.summary.totalAlerts)]);
    rows.push(['Critical', String(data.summary.criticalCount)]);
    rows.push(['High', String(data.summary.highCount)]);
    rows.push(['Medium', String(data.summary.mediumCount)]);
    rows.push(['Low', String(data.summary.lowCount)]);
    rows.push([]);
    
    // Alerts
    if (data.tables.alerts.length > 0) {
        rows.push(['ALERTS']);
        const headers = Object.keys(data.tables.alerts[0]);
        rows.push(headers);
        data.tables.alerts.forEach(alert => {
            rows.push(headers.map(h => String(alert[h] ?? '')));
        });
        rows.push([]);
    }
    
    // Convert to CSV string
    const csv = rows.map(row => row.map(cell => `"${String(cell).replace(/"/g, '""')}"`).join(',')).join('\n');
    
    const blob = new Blob([csv], { type: 'text/csv' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `EDR-Report-${template}-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// Export to JSON
async function exportToJSON(data: ReportData, template: ReportTemplate): Promise<void> {
    const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `EDR-Report-${template}-${new Date().toISOString().slice(0, 10)}.json`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
}

// Generate HTML report
function generateHTMLReport(data: ReportData, template: string, title: string, forWord = false): string {
    const severityColors: Record<string, string> = {
        critical: '#ef4444',
        high: '#f97316',
        medium: '#f59e0b',
        low: '#3b82f6',
    };
    
    const html = `
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>${title}</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            line-height: 1.6;
            color: #334155;
            max-width: 1200px;
            margin: 0 auto;
            padding: 40px;
            background: #fff;
        }
        ${forWord ? `
        body { font-family: 'Calibri', sans-serif; }
        ` : ''}
        .header {
            text-align: center;
            border-bottom: 3px solid #0891b2;
            padding-bottom: 20px;
            margin-bottom: 30px;
        }
        .header h1 {
            color: #0891b2;
            margin: 0 0 10px 0;
            font-size: 28px;
        }
        .meta {
            color: #64748b;
            font-size: 14px;
        }
        .summary-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }
        .kpi-card {
            background: linear-gradient(135deg, #f1f5f9 0%, #e2e8f0 100%);
            border-radius: 8px;
            padding: 20px;
            text-align: center;
        }
        .kpi-value {
            font-size: 32px;
            font-weight: bold;
            color: #0891b2;
        }
        .kpi-label {
            color: #64748b;
            font-size: 14px;
            text-transform: uppercase;
            letter-spacing: 0.5px;
        }
        .section {
            margin-bottom: 40px;
        }
        .section h2 {
            color: #0f172a;
            border-left: 4px solid #0891b2;
            padding-left: 15px;
            margin-bottom: 20px;
        }
        table {
            width: 100%;
            border-collapse: collapse;
            margin-bottom: 20px;
        }
        th {
            background: #f1f5f9;
            padding: 12px;
            text-align: left;
            font-weight: 600;
            color: #0f172a;
            border-bottom: 2px solid #e2e8f0;
        }
        td {
            padding: 10px 12px;
            border-bottom: 1px solid #e2e8f0;
        }
        tr:hover {
            background: #f8fafc;
        }
        .severity {
            display: inline-block;
            padding: 4px 8px;
            border-radius: 4px;
            font-size: 12px;
            font-weight: 600;
            text-transform: uppercase;
        }
        .severity-critical { background: #fee2e2; color: #991b1b; }
        .severity-high { background: #ffedd5; color: #9a3412; }
        .severity-medium { background: #fef3c7; color: #92400e; }
        .severity-low { background: #dbeafe; color: #1e40af; }
        
        .chart-container {
            background: #f8fafc;
            border-radius: 8px;
            padding: 20px;
            margin-bottom: 20px;
        }
        
        @media print {
            body { padding: 20px; }
            .no-print { display: none; }
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>${title}</h1>
        <p class="meta">
            Generated: ${new Date(data.generatedAt).toLocaleString()}<br>
            Period: ${new Date(data.period.from).toLocaleDateString()} - ${new Date(data.period.to).toLocaleDateString()}
        </p>
    </div>
    
    <div class="summary-grid">
        <div class="kpi-card">
            <div class="kpi-value">${data.summary.totalAlerts}</div>
            <div class="kpi-label">Total Alerts</div>
        </div>
        <div class="kpi-card">
            <div class="kpi-value" style="color: ${severityColors.critical}">${data.summary.criticalCount}</div>
            <div class="kpi-label">Critical</div>
        </div>
        <div class="kpi-card">
            <div class="kpi-value" style="color: ${severityColors.high}">${data.summary.highCount}</div>
            <div class="kpi-label">High</div>
        </div>
        <div class="kpi-card">
            <div class="kpi-value" style="color: ${severityColors.medium}">${data.summary.mediumCount}</div>
            <div class="kpi-label">Medium</div>
        </div>
        <div class="kpi-card">
            <div class="kpi-value">${data.summary.totalDevices}</div>
            <div class="kpi-label">Devices</div>
        </div>
    </div>
    
    <div class="section">
        <h2>Recent Alerts</h2>
        <table>
            <thead>
                <tr>
                    <th>Time</th>
                    <th>Severity</th>
                    <th>Rule</th>
                    <th>Endpoint</th>
                </tr>
            </thead>
            <tbody>
                ${data.tables.alerts.slice(0, 50).map(a => `
                <tr>
                    <td>${new Date(a.timestamp).toLocaleString()}</td>
                    <td><span class="severity severity-${a.severity}">${a.severity}</span></td>
                    <td>${a.rule_title || 'N/A'}</td>
                    <td>${a.agent_hostname || a.agent_id?.slice(0, 8) || 'N/A'}</td>
                </tr>
                `).join('')}
            </tbody>
        </table>
        ${data.tables.alerts.length > 50 ? `<p style="color: #64748b; font-size: 14px;">+ ${data.tables.alerts.length - 50} more alerts in full report</p>` : ''}
    </div>
    
    <div class="section">
        <h2>Endpoint Summary</h2>
        <table>
            <thead>
                <tr>
                    <th>Hostname</th>
                    <th>OS</th>
                    <th>Status</th>
                    <th>Health Score</th>
                </tr>
            </thead>
            <tbody>
                ${data.tables.devices.slice(0, 30).map(d => `
                <tr>
                    <td>${d.hostname || 'N/A'}</td>
                    <td>${d.os_type || 'N/A'}</td>
                    <td>${d.status || 'N/A'}</td>
                    <td>${d.health_score || 'N/A'}%</td>
                </tr>
                `).join('')}
            </tbody>
        </table>
        ${data.tables.devices.length > 30 ? `<p style="color: #64748b; font-size: 14px;">+ ${data.tables.devices.length - 30} more devices in full report</p>` : ''}
    </div>
    
    <div class="section no-print" style="margin-top: 40px; padding-top: 20px; border-top: 1px solid #e2e8f0;">
        <p style="color: #94a3b8; font-size: 12px; text-align: center;">
            EDR Security Platform • Report ID: ${Date.now()} • Template: ${template}
        </p>
    </div>
</body>
</html>`;
    
    return html;
}
