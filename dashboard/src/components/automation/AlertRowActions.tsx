import { useState } from 'react';
import { Button } from '../ui/Button';
import { Settings, Play, Zap, ArrowRight, Clock, CheckCircle } from 'lucide-react';

interface Alert {
  id: string;
  severity: string;
  ruleName: string;
  agentId: string;
  title: string;
  description?: string;
  riskScore?: number;
  status: string;
}

interface AlertRowActionsProps {
  alert: Alert;
  onNavigateToAutomation: (alert: Alert) => void;
  onNavigateToPlaybooks: (alert: Alert) => void;
  onQuickExecute: (alert: Alert) => void;
}

export function AlertRowActions({
  alert,
  onNavigateToAutomation,
  onNavigateToPlaybooks,
  onQuickExecute,
}: AlertRowActionsProps) {
  const [isExecuting, setIsExecuting] = useState(false);

  const handleQuickExecute = async () => {
    setIsExecuting(true);
    try {
      await onQuickExecute(alert);
    } finally {
      setIsExecuting(false);
    }
  };

  return (
    <div className="flex items-center gap-2">
      {/* زر الانتقال إلى الأتمتة */}
      <Button
        variant="outline"
        size="sm"
        onClick={() => onNavigateToAutomation(alert)}
        className="flex items-center gap-1 hover:bg-blue-50 dark:hover:bg-blue-900/20"
        title="الانتقال إلى صفحة الأتمتة مع تفاصيل هذا التنبيه"
      >
        <Settings className="w-3 h-3" />
        <span className="hidden sm:inline">الأتمتة</span>
        <ArrowRight className="w-3 h-3" />
      </Button>

      {/* زر الانتقال إلى كتيبات الإجراءات */}
      <Button
        variant="outline"
        size="sm"
        onClick={() => onNavigateToPlaybooks(alert)}
        className="flex items-center gap-1 hover:bg-green-50 dark:hover:bg-green-900/20"
        title="الانتقال إلى صفحة كتيبات الإجراءات مع تفاصيل هذا التنبيه"
      >
        <Play className="w-3 h-3" />
        <span className="hidden sm:inline">كتيبات</span>
        <ArrowRight className="w-3 h-3" />
      </Button>

      {/* زر التنفيذ السريع للتنبيهات الحرجة */}
      {alert.severity === 'critical' && (
        <Button
          variant="destructive"
          size="sm"
          onClick={handleQuickExecute}
          disabled={isExecuting}
          className="flex items-center gap-1"
          title="تنفيذ احتواء سريع للتهديد الحرج"
        >
          {isExecuting ? (
            <>
              <div className="w-3 h-3 border-2 border-white border-t-transparent rounded-full animate-spin" />
              <span className="hidden sm:inline">جاري التنفيذ</span>
            </>
          ) : (
            <>
              <Zap className="w-3 h-3" />
              <span className="hidden sm:inline">احتواء</span>
            </>
          )}
        </Button>
      )}

      {/* مؤشر الحالة */}
      {alert.status === 'resolved' && (
        <div className="flex items-center gap-1 text-xs text-green-600 dark:text-green-400">
          <CheckCircle className="w-3 h-3" />
          <span className="hidden sm:inline">تم الحل</span>
        </div>
      )}

      {alert.status === 'in_progress' && (
        <div className="flex items-center gap-1 text-xs text-blue-600 dark:text-blue-400">
          <Clock className="w-3 h-3" />
          <span className="hidden sm:inline">قيد المعالجة</span>
        </div>
      )}
    </div>
  );
}
