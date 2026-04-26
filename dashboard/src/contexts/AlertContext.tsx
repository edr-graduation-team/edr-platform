import { createContext, useContext, useState } from 'react';
import type { ReactNode } from 'react';

interface AlertDetails {
  severity: string;
  ruleName: string;
  agentId: string;
  title: string;
  description?: string;
  riskScore?: number;
}

interface AlertContext {
  alertId: string;
  alertDetails: AlertDetails;
  timestamp: string;
}

interface AlertContextType {
  alertContext: AlertContext | null;
  setAlertContext: (context: AlertContext | null) => void;
  clearAlertContext: () => void;
}

const AlertContextProvider = createContext<AlertContextType | undefined>(undefined);

export function useAlertContext() {
  const context = useContext(AlertContextProvider);
  if (context === undefined) {
    throw new Error('useAlertContext must be used within an AlertContextProvider');
  }
  return context;
}

interface AlertContextProviderProps {
  children: ReactNode;
}

export function AlertContextProviderComponent({ children }: AlertContextProviderProps) {
  const [alertContext, setAlertContext] = useState<AlertContext | null>(null);

  const clearAlertContext = () => {
    setAlertContext(null);
  };

  return (
    <AlertContextProvider.Provider value={{ alertContext, setAlertContext, clearAlertContext }}>
      {children}
    </AlertContextProvider.Provider>
  );
}
