import { PieChart, Pie, Cell, Tooltip, ResponsiveContainer } from 'recharts';

const OS_COLORS: Record<string, string> = {
    windows: '#38bdf8',
    linux: '#a855f7',
    macos: '#10b981',
};
const OS_FALLBACK = '#64748b';

interface OSDonutProps {
    byOsType: Record<string, number>;
}

export function OSDonut({ byOsType }: OSDonutProps) {
    const data = Object.entries(byOsType)
        .filter(([, v]) => v > 0)
        .map(([k, v]) => ({ name: k.charAt(0).toUpperCase() + k.slice(1), value: v, key: k }));

    if (data.length === 0) {
        return (
            <div className="flex items-center justify-center h-32 text-slate-500 text-sm">
                No OS data available
            </div>
        );
    }

    const total = data.reduce((s, d) => s + d.value, 0);

    return (
        <div className="flex flex-col items-center gap-2">
            <ResponsiveContainer width="100%" height={130}>
                <PieChart>
                    <Pie
                        data={data}
                        dataKey="value"
                        nameKey="name"
                        cx="50%"
                        cy="50%"
                        innerRadius={38}
                        outerRadius={58}
                        paddingAngle={3}
                        strokeWidth={0}
                    >
                        {data.map((entry) => (
                            <Cell key={entry.key} fill={OS_COLORS[entry.key] || OS_FALLBACK} />
                        ))}
                    </Pie>
                    <Tooltip
                        contentStyle={{
                            background: 'rgba(15,23,42,0.95)',
                            border: '1px solid rgba(30,48,72,0.8)',
                            borderRadius: '10px',
                            color: 'white',
                            fontSize: '12px',
                            fontFamily: 'Inter, sans-serif',
                        }}
                        formatter={(v: number | undefined) => [`${v ?? 0} agents`, '']}
                    />
                </PieChart>
            </ResponsiveContainer>
            {/* Legend */}
            <div className="flex flex-wrap justify-center gap-3">
                {data.map((d) => (
                    <div
                        key={d.key}
                        className="flex items-center gap-1.5 text-[11px] font-medium text-slate-500 dark:text-slate-400"
                    >
                        <span
                            className="w-2.5 h-2.5 rounded-full shrink-0"
                            style={{ background: OS_COLORS[d.key] || OS_FALLBACK }}
                        />
                        {d.name} ({Math.round((d.value / total) * 100)}%)
                    </div>
                ))}
            </div>
        </div>
    );
}
