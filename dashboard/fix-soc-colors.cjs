const fs = require('fs');
const path = require('path');

const pagesDir = path.join(__dirname, 'src', 'pages');

function processDir(dir) {
    const files = fs.readdirSync(dir);
    for (const file of files) {
        const fullPath = path.join(dir, file);
        const stat = fs.statSync(fullPath);
        if (stat.isDirectory()) {
            processDir(fullPath);
        } else if (fullPath.endsWith('.tsx') || fullPath.endsWith('.ts')) {
            let content = fs.readFileSync(fullPath, 'utf8');
            let original = content;

            // Replace gray with slate
            content = content.replace(/gray-/g, 'slate-');
            
            // Replace old card design with new standard design
            // Common old card class: rounded-xl border border-gray-200 dark:border-gray-700 bg-white dark:bg-gray-900/40
            // New is: rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm
            content = content.replace(/rounded-xl border border-slate-200 dark:border-slate-700 bg-white dark:bg-slate-900\/40/g, 'rounded-xl border border-slate-200 dark:border-slate-700/60 bg-white/95 dark:bg-slate-800/90 backdrop-blur-sm shadow-sm');

            // Apply slide-up animation to main wrappers
            content = content.replace(/<div className="space-y-5">/g, '<div className="space-y-5 animate-slide-up-fade">');
            content = content.replace(/<div className="space-y-4">/g, '<div className="space-y-4 animate-slide-up-fade">');

            // Fix specific header elements
            content = content.replace(/<h1 className="text-xl font-bold/g, '<h2 className="text-lg font-bold');
            content = content.replace(/<h1 className="text-3xl font-bold/g, '<h2 className="text-xl font-bold');
            content = content.replace(/<\/h1>/g, '<\/h2>');

            if (content !== original) {
                fs.writeFileSync(fullPath, content, 'utf8');
                console.log(`Updated ${fullPath}`);
            }
        }
    }
}

processDir(pagesDir);
console.log('Done.');
