#!/usr/bin/env node
// Inject custom tag colors and server log link into Playwright HTML report.
// Usage: node e2e/inject-tag-colors.cjs

const fs = require('fs');
const path = require('path');

const reportPath = path.join(__dirname, '..', 'playwright-report', 'index.html');
if (!fs.existsSync(reportPath)) process.exit(0);

let html = fs.readFileSync(reportPath, 'utf8');
if (html.includes('pwCustom')) process.exit(0);

// Inline script: style screenshot tags + add server log link to header
const injection = `<script id="pwCustom">
(()=>{
  // Style screenshot tags
  new MutationObserver(()=>document.querySelectorAll('.label').forEach(e=>{
    if(e.textContent==='screenshot')Object.assign(e.style,{backgroundColor:'#7c3aed',color:'#fff',border:'1px solid #6d28d9'})
  })).observe(document.body,{childList:1,subtree:1});
  // Add server log link
  const addLink=()=>{
    const header=document.querySelector('.header-view');
    if(!header||header.querySelector('.server-log-link'))return;
    const link=document.createElement('a');
    link.href='server.log';
    link.className='server-log-link';
    link.textContent='Server Log';
    Object.assign(link.style,{marginLeft:'auto',padding:'4px 12px',background:'#1e1e1e',color:'#fff',borderRadius:'4px',textDecoration:'none',fontSize:'13px'});
    link.onmouseover=()=>link.style.background='#333';
    link.onmouseout=()=>link.style.background='#1e1e1e';
    header.style.display='flex';
    header.style.alignItems='center';
    header.appendChild(link);
  };
  new MutationObserver(addLink).observe(document.body,{childList:1,subtree:1});
  addLink();
})()
</script>`;

fs.writeFileSync(reportPath, html.replace('</body>', injection + '</body>'));
