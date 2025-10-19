async function loadTables() {
    const container = document.getElementById('tables-container');
    try {
        const jobsRes = await fetch('/api/v1/renovatejobs');
        if (!jobsRes.ok) throw new Error('Failed to fetch renovate jobs');
        const jobsList = await jobsRes.json();
        if (typeof jobsList !== 'object' || jobsList === null) throw new Error('Invalid jobs response');

        const sortedJobs = jobsList.sort((a, b) => (a.name || '').localeCompare(b.name || ''));

        // Build a map of existing sections for in-place updates
        const existingSections = new Map();
        for (const s of container.querySelectorAll('details.renovatejob')) {
            // class names are `${name}-${namespace}` as used in createTableSection
            const key = s.className.split(' ').find(c => c !== 'renovatejob');
            if (key) existingSections.set(key, s);
        }

        for (const jobData of sortedJobs) {
            const key = `${jobData.name}-${jobData.namespace}`;
            const existing = existingSections.get(key);

            // Create fresh tbody for diffing rows
            const newTbody = document.createElement('tbody');
            if (Array.isArray(jobData.projects)) {
                const statusOrder = { running: 0, scheduled: 1, failed: 2, completed: 3 };
                const sortedProjects = [...jobData.projects].sort((a, b) => {
                    const aStatus = (a.status || '').toLowerCase();
                    const bStatus = (b.status || '').toLowerCase();
                    const aOrder = statusOrder.hasOwnProperty(aStatus) ? statusOrder[aStatus] : 99;
                    const bOrder = statusOrder.hasOwnProperty(bStatus) ? statusOrder[bStatus] : 99;
                    if (aOrder !== bOrder) return aOrder - bOrder;
                    return (a.name || '').localeCompare(b.name || '');
                });
                for (const projectStatus of sortedProjects) {
                    const row = document.createElement('tr');
                    row.appendChild(getNameRowItem(projectStatus));
                    row.appendChild(getStatusRowItem(projectStatus));
                    row.appendChild(getActionRowItem(projectStatus, jobData));
                    newTbody.appendChild(row);
                }
            } else {
                const row = document.createElement('tr');
                row.innerHTML = `<td>${jobData.name}</td><td>${jobData.namespace}</td><td>-</td><td>-</td><td></td>`;
                newTbody.appendChild(row);
            }

            if (existing) {
                // Update caption (summary) text if changed
                const summary = existing.querySelector('summary');
                const expectedCaption = `${jobData.name} - ${jobData.namespace}`;
                if (summary && summary.firstChild && summary.firstChild.nodeValue !== expectedCaption) {
                    summary.firstChild.nodeValue = expectedCaption;
                }

                // Replace only the tbody in the existing table to avoid flicker
                const table = existing.querySelector('table');
                if (table) {
                    const oldTbody = table.querySelector('tbody');
                    if (oldTbody) table.replaceChild(newTbody, oldTbody);
                    else table.appendChild(newTbody);
                }

                // Update discovery button state asynchronously
                const discoveryBtn = existing.querySelector('button.discovery-btn');
                if (discoveryBtn) updateDiscoveryButton(discoveryBtn, jobData);

                // remove from map to mark as handled
                existingSections.delete(key);
            } else {
                // create new section
                const section = createTableSection(jobData, newTbody);
                container.appendChild(section);
            }
        }

        // Remove any sections that no longer exist in the data
        for (const s of existingSections.values()) {
            s.remove();
        }
    } catch (err) {
        container.innerHTML = `<p style="color:red;">${err.message}</p>`;
    }
}

document.addEventListener('DOMContentLoaded', () => {
    loadTables();
    setInterval(loadTables, 30000); // reload every 30 seconds
});

async function updateDiscoveryButton(discoveryBtn, jobData) {
    try {
        const res = await fetch(`/api/v1/discovery/status?renovate=${encodeURIComponent(jobData.name)}&namespace=${encodeURIComponent(jobData.namespace)}`);
        if (res.ok) {
            const data = await res.json();
            if (data.status === 'running') {
                discoveryBtn.disabled = true;
                discoveryBtn.textContent = 'Discovery Running...';
                discoveryBtn.style.backgroundColor = '#2196f3';
            } else {
                discoveryBtn.disabled = false;
                discoveryBtn.textContent = 'Run Discovery';
                discoveryBtn.style.backgroundColor = '';
            }
        } else {
            discoveryBtn.disabled = false;
            discoveryBtn.textContent = 'Run Discovery';
            discoveryBtn.style.backgroundColor = '';
        }
    } catch (e) {
        discoveryBtn.disabled = false;
        discoveryBtn.textContent = 'Run Discovery';
        discoveryBtn.style.backgroundColor = '';
    }
}
function createTableSection(jobData, tbody) {
    const section = document.createElement('details');
    section.classList.add(`${jobData.name}-${jobData.namespace}`)
    section.classList.add('renovatejob')
    section.open = true

    const caption = document.createElement('summary')
    caption.innerText = `${jobData.name} - ${jobData.namespace}`;

    const discoveryBtn = document.createElement('button')
    discoveryBtn.classList.add('discovery-btn');
    discoveryBtn.textContent = 'Run Discovery';
    updateDiscoveryButton(discoveryBtn, jobData);


    discoveryBtn.onclick = async () => {
        discoveryBtn.disabled = true;
        discoveryBtn.textContent = 'Running...';
        try {
            const res = await fetch('/api/v1/discovery/start', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    renovateJob: jobData.name,
                    namespace: jobData.namespace,
                })
            });
            if (res.ok) {
                discoveryBtn.textContent = 'Discovery Started!';
                discoveryBtn.style.backgroundColor = '#4caf50';
                // Reload tables after successful trigger
                setTimeout(loadTables, 500);
            } else {
                discoveryBtn.textContent = 'Failed';
                discoveryBtn.style.backgroundColor = '#f44336';
            }
        } catch (e) {
            discoveryBtn.textContent = 'Error';
            discoveryBtn.style.backgroundColor = '#f44336';
        }
        setTimeout(() => {
            discoveryBtn.textContent = 'Run Discovery';
            discoveryBtn.style.backgroundColor = '';
        }, 2000);
    }

    caption.appendChild(discoveryBtn)
    section.appendChild(caption)

    const table = document.createElement('table')
    section.appendChild(table)

    const thead = document.createElement('thead');
    thead.innerHTML = `<tr><th>Project</th><th>Status</th><th>Action</th></tr>`;
    table.appendChild(thead);
    table.appendChild(tbody);

    return section
}

function getNameRowItem(projectStatus) {
    const div = document.createElement("div")
    div.innerText = projectStatus.name || '-'

    const td = document.createElement("td")
    td.appendChild(div)
    td.classList.add("name")
    return td
}

function getStatusRowItem(projectStatus) {

    const div = document.createElement("div")
    div.innerText = projectStatus.status || '-'

    const td = document.createElement("td")
    td.appendChild(div)
    td.classList.add("status");
    td.classList.add(projectStatus.status);

    return td
}

function getActionRowItem(projectStatus, jobData) {
    const actionTd = document.createElement('td');
    const btn = document.createElement('button');
    btn.classList.add('trigger-btn');
    btn.textContent = 'Trigger';
    btn.onclick = async () => {
        btn.disabled = true;
        btn.textContent = 'Triggering...';
        try {
            const res = await fetch('/api/v1/renovate', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    renovateJob: jobData.name,
                    namespace: jobData.namespace,
                    project: projectStatus.name
                })
            });
            if (res.ok) {
                btn.textContent = 'Triggered!';
                btn.style.backgroundColor = '#4caf50';
                // Reload tables after successful trigger
                setTimeout(loadTables, 500);
            } else {
                btn.textContent = 'Failed';
                btn.style.backgroundColor = '#f44336';
            }
        } catch (e) {
            btn.textContent = 'Error';
            btn.style.backgroundColor = '#f44336';
        }
        setTimeout(() => {
            btn.disabled = false;
            btn.textContent = 'Trigger';
            btn.style.backgroundColor = '';
        }, 2000);
    };
    actionTd.appendChild(btn);

    const logRedirect = document.createElement('a')
    logRedirect.classList.add('log-btn');
    logRedirect.href = `/api/v1/logs?renovate=${encodeURIComponent(jobData.name)}&namespace=${encodeURIComponent(jobData.namespace)}&project=${encodeURIComponent(projectStatus.name)}`
    logRedirect.target = "_blank"
    logRedirect.innerText = "Logs"

    actionTd.appendChild(logRedirect)

    return actionTd
}
