import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { LogIn, AlertCircle } from 'lucide-react';
import { authApi } from '../api/client';
import ProtocolLogo from '../components/ProtocolLogo';

export default function Login() {
    const navigate = useNavigate();
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');

    const handleSubmit = async (e: React.FormEvent) => {
        e.preventDefault();
        setError('');
        setLoading(true);

        try {
            // Use the real auth API — sends to Connection Manager with correct field names
            await authApi.login(username, password);
            navigate('/');
        } catch (err: any) {
            setError(err.response?.data?.message || 'Invalid username or password');
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="min-h-screen flex items-center justify-center bg-slate-950 px-4 relative overflow-hidden">
            {/* Subtle glowing radial background */}
            <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[800px] h-[800px] pointer-events-none" style={{ background: 'radial-gradient(circle, rgba(22,78,99,0.2) 0%, transparent 70%)' }}></div>

            <div className="max-w-md w-full relative z-10">
                {/* Logo */}
                <div className="flex justify-center items-center mb-10 w-full">
                    <div className="flex items-center justify-center gap-5">
                        <ProtocolLogo className="w-24 h-24 shrink-0 drop-shadow-[0_0_15px_rgba(6,182,212,0.4)]" idPrefix="login" />
                        
                        {/* Typography */}
                        <div className="flex flex-col items-start justify-center border-l border-slate-700/50 pl-5">
                            <span className="text-cyan-400 text-xs font-bold tracking-[0.3em] uppercase mb-1">Protocol Soft</span>
                            <div className="flex items-baseline gap-2">
                                <span className="text-4xl font-extrabold text-transparent bg-clip-text bg-gradient-to-r from-white to-slate-400 tracking-tight uppercase">EDR</span>
                                <span className="text-4xl font-light text-white uppercase">Platform</span>
                            </div>
                        </div>
                    </div>
                </div>

                {/* Login Form */}
                <div className="bg-slate-900/60 backdrop-blur-md border border-slate-700/50 rounded-2xl shadow-2xl shadow-cyan-900/20 p-8">
                    <h2 className="text-xl font-semibold text-white mb-6 text-center">
                        Authenticate Session
                    </h2>

                    {error && (
                        <div className="mb-6 p-4 bg-red-900/30 border border-red-500/50 rounded-lg flex items-center gap-3 text-red-400 text-sm font-medium">
                            <AlertCircle className="w-5 h-5 shrink-0" />
                            <span>{error}</span>
                        </div>
                    )}

                    <form onSubmit={handleSubmit} className="space-y-5">
                        <div>
                            <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider mb-2">
                                Username
                            </label>
                            <input
                                type="text"
                                value={username}
                                onChange={(e) => setUsername(e.target.value)}
                                className="w-full bg-slate-800/50 border border-slate-700/50 rounded-lg px-4 py-3 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 transition-all font-medium"
                                placeholder="Enter Username"
                                required
                            />
                        </div>

                        <div>
                            <div className="flex items-center justify-between mb-2">
                                <label className="block text-xs font-semibold text-slate-400 uppercase tracking-wider">
                                    Password
                                </label>
                                <a href="#" className="text-xs font-semibold text-cyan-400 hover:text-cyan-300 transition-colors">
                                    Reset Access?
                                </a>
                            </div>
                            <input
                                type="password"
                                value={password}
                                onChange={(e) => setPassword(e.target.value)}
                                className="w-full bg-slate-800/50 border border-slate-700/50 rounded-lg px-4 py-3 text-white placeholder-slate-500 focus:outline-none focus:ring-2 focus:ring-cyan-500/50 focus:border-cyan-500 transition-all font-medium"
                                placeholder="••••••••"
                                required
                            />
                        </div>

                        <div className="flex items-center">
                            <label className="flex items-center cursor-pointer group">
                                <input type="checkbox" className="w-4 h-4 rounded border-slate-600 bg-slate-800 text-cyan-500 focus:ring-cyan-500 focus:ring-offset-slate-900 cursor-pointer" />
                                <span className="ml-2 text-sm font-medium text-slate-400 group-hover:text-slate-300 transition-colors">Remember node session</span>
                            </label>
                        </div>

                        <button
                            type="submit"
                            disabled={loading}
                            className="w-full bg-gradient-to-r from-blue-600 to-cyan-600 hover:from-blue-500 hover:to-cyan-500 text-white font-bold py-3.5 px-4 rounded-lg flex items-center justify-center gap-2 hover:scale-[1.02] active:scale-[0.98] transition-all duration-200 shadow-[0_0_20px_rgba(6,182,212,0.3)] disabled:opacity-50 disabled:cursor-not-allowed disabled:hover:scale-100 mt-4"
                        >
                            {loading ? (
                                <div className="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin" />
                            ) : (
                                <>
                                    <LogIn className="w-5 h-5" />
                                    Secure Login
                                </>
                            )}
                        </button>
                    </form>
                </div>

                {/* Footer */}
                <p className="text-center text-xs font-medium text-slate-500 mt-8 tracking-widest uppercase">
                    EDR Platform v1.0.0 • Secure Node
                </p>
            </div>
        </div>
    );
}
