#!/usr/bin/env node
// Inject custom tag colors into Playwright HTML report.
// Usage: node e2e/inject-tag-colors.cjs

const fs = require('fs');
const path = require('path');

const reportPath = path.join(__dirname, '..', 'playwright-report', 'index.html');
if (!fs.existsSync(reportPath)) process.exit(0);

let html = fs.readFileSync(reportPath, 'utf8');
if (html.includes('pwTagColors')) process.exit(0);

// Minimal inline script - styles tags by text content
const injection = `<script id="pwTagColors">
(o=>new MutationObserver(()=>document.querySelectorAll('.label').forEach(e=>{
if(e.textContent==='screenshot')Object.assign(e.style,{backgroundColor:'#7c3aed',color:'#fff',border:'1px solid #6d28d9'})
})).observe(document.body,{childList:1,subtree:1}))()
</script>`;

fs.writeFileSync(reportPath, html.replace('</body>', injection + '</body>'));
