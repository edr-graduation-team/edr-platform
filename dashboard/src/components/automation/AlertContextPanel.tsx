import { Button } from '../ui/Button';
import { Badge } from '../ui/Badge';
import { Card } from '../ui/Card';
import { X, AlertTriangle, User } from 'lucide-react';

interface AlertDetails {
  severity: string;
  ruleName: string;
  agentId: string;
  title: string;
  description?: string;
  riskScore?: number;
  contextSnapshot?: any;
}


interface AlertContextPanelProps {
  alertId: string;
  alertDetails: AlertDetails;
  onClearContext: () => void;
}

export function AlertContextPanel({ alertId, alertDetails, onClearContext }: AlertContextPanelProps) {
  const getSeverityColor = (severity: string) => {
    switch (severity) {
      case 'critical': return 'bg-rose-100 text-rose-800 dark:bg-rose-900/20 dark:text-rose-400';
      case 'high': return 'bg-orange-100 text-orange-800 dark:bg-orange-900/20 dark:text-orange-400';
      case 'medium': return 'bg-amber-100 text-amber-800 dark:bg-amber-900/20 dark:text-amber-400';
      default: return 'bg-blue-100 text-blue-800 dark:bg-blue-900/20 dark:text-blue-400';
    }
  };

  return (
    <Card className="border-indigo-200 dark:border-indigo-800 bg-indigo-50 dark:bg-indigo-900/20 shadow-sm">
      <div className="flex items-center justify-between p-4 border-b border-indigo-200 dark:border-indigo-800">
        <div className="flex items-center gap-2">
          <AlertTriangle className="w-5 h-5 text-indigo-600 dark:text-indigo-400" />
          <h3 className="font-semibold text-indigo-900 dark:text-indigo-100">
            Active Alert Context
          </h3>
        </div>
        <Button variant="ghost" size="sm" onClick={onClearContext}>
          <X className="w-4 h-4" />
        </Button>
      </div>
      
      <div className="p-6 space-y-5">
        {/* Basic Info */}
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-6">
          <div>
            <div className="text-xs uppercase tracking-wider font-bold text-indigo-700 dark:text-indigo-300 mb-1.5">Alert ID</div>
            <div className="font-mono text-sm text-indigo-900 dark:text-indigo-100 bg-indigo-100 dark:bg-indigo-900/30 px-2.5 py-1 rounded-md">
              {alertId.substring(0, 8)}...
            </div>
          </div>
          
          <div>
            <div className="text-xs uppercase tracking-wider font-bold text-indigo-700 dark:text-indigo-300 mb-1.5">Severity</div>
            <Badge className={`${getSeverityColor(alertDetails.severity)} uppercase text-[10px] tracking-wider font-bold`}>
              {alertDetails.severity}
            </Badge>
          </div>
          
          <div>
            <div className="text-xs uppercase tracking-wider font-bold text-indigo-700 dark:text-indigo-300 mb-1.5">Rule Triggered</div>
            <div className="font-medium text-sm text-indigo-900 dark:text-indigo-100 truncate" title={alertDetails.ruleName}>
              {alertDetails.ruleName}
            </div>
          </div>
          
          <div>
            <div className="text-xs uppercase tracking-wider font-bold text-indigo-700 dark:text-indigo-300 mb-1.5">Target Agent</div>
            <div className="flex items-center gap-1.5 text-indigo-900 dark:text-indigo-100 bg-indigo-100 dark:bg-indigo-900/30 px-2.5 py-1 rounded-md w-fit">
              <User className="w-3.5 h-3.5" />
              <span className="font-mono text-sm">{alertDetails.agentId.substring(0, 8)}...</span>
            </div>
          </div>
        </div>
        
        {/* Additional Details */}
        <div className="space-y-4">
          <div>
            <div className="text-xs uppercase tracking-wider font-bold text-indigo-700 dark:text-indigo-300 mb-1.5">Title</div>
            <div className="font-medium text-sm text-indigo-900 dark:text-indigo-100">
              {alertDetails.title}
            </div>
          </div>
          
          {alertDetails.description && (
            <div>
              <div className="text-xs uppercase tracking-wider font-bold text-indigo-700 dark:text-indigo-300 mb-1.5">Description</div>
              <div className="text-sm text-indigo-900 dark:text-indigo-100 bg-indigo-100/50 dark:bg-indigo-900/30 p-3 rounded-lg border border-indigo-200/50 dark:border-indigo-800/50">
                {alertDetails.description}
              </div>
            </div>
          )}
          
          {alertDetails.riskScore && (
            <div>
              <div className="text-xs uppercase tracking-wider font-bold text-indigo-700 dark:text-indigo-300 mb-1.5">Risk Score</div>
              <div className="flex items-center gap-3">
                <div className="flex-1 bg-indigo-200/50 dark:bg-indigo-800/50 rounded-full h-2.5 max-w-md">
                  <div 
                    className="bg-indigo-600 dark:bg-indigo-400 h-2.5 rounded-full"
                    style={{ width: `${Math.min(alertDetails.riskScore, 100)}%` }}
                  />
                </div>
                <span className="text-sm font-bold text-indigo-900 dark:text-indigo-100">
                  {alertDetails.riskScore}/100
                </span>
              </div>
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}
