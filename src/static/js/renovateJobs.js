document.addEventListener('alpine:init', () => {
    Alpine.store('toastStore', {
        toasts: [],
        nextId: 1,

        addToast(type, title, message = '', duration = 3000) {
            const id = this.nextId++;
            const toast = { id, type, title, message, visible: true };
            this.toasts.push(toast);
            setTimeout(() => this.removeToast(id), duration);
        },

        removeToast(id) {
            const index = this.toasts.findIndex(t => t.id === id);
            if (index !== -1) {
                this.toasts[index].visible = false;
                setTimeout(() => {
                    this.toasts = this.toasts.filter(t => t.id !== id);
                }, 300);
            }
        },

        success(title, message = '') {
            this.addToast('success', title, message);
        },

        error(title, message = '') {
            this.addToast('error', title, message);
        },

        info(title, message = '') {
            this.addToast('info', title, message);
        }
    });

    Alpine.store('dashboard', {
        jobs: [],
        loading: true,
        error: null,
        stats: {
            scheduled: 0,
            running: 0,
            completed: 0,
            failed: 0
        },

        init() {
            this.loadJobs();
            setInterval(() => this.loadJobs(), 30000);
        },

        async loadJobs() {
            try {
                this.loading = true;
                this.error = null;

                const response = await fetch('/api/v1/renovatejobs');
                if (!response.ok) throw new Error(`HTTP ${response.status}: Failed to fetch jobs`);

                const data = await response.json();
                if (typeof data !== 'object' || data === null) throw new Error('Invalid response format');

                this.jobs = Array.isArray(data) ? data.sort((a, b) =>
                    (a.name || '').localeCompare(b.name || '')
                ) : [];

                for (const job of this.jobs) {
                    await this.updateDiscoveryStatus(job);
                    if (Array.isArray(job.projects)) {
                        job.projects = this.sortProjects(job.projects);
                        for (const project of job.projects) {
                            project.triggering = false;
                        }
                    }
                }

                this.calculateStats();
            } catch (err) {
                console.error('Error loading jobs:', err);
                this.error = err.message;
                Alpine.store('toastStore').error('Failed to load jobs', err.message);
            } finally {
                this.loading = false;
            }
        },

        sortProjects(projects) {
            const statusOrder = { running: 0, scheduled: 1, failed: 2, completed: 3 };
            return [...projects].sort((a, b) => {
                const aStatus = (a.status || '').toLowerCase();
                const bStatus = (b.status || '').toLowerCase();
                const aOrder = statusOrder.hasOwnProperty(aStatus) ? statusOrder[aStatus] : 99;
                const bOrder = statusOrder.hasOwnProperty(bStatus) ? statusOrder[bStatus] : 99;
                if (aOrder !== bOrder) return aOrder - bOrder;
                return (a.name || '').localeCompare(b.name || '');
            });
        },

        calculateStats() {
            this.stats = { scheduled: 0, running: 0, completed: 0, failed: 0 };
            for (const job of this.jobs) {
                if (Array.isArray(job.projects)) {
                    for (const project of job.projects) {
                        const status = (project.status || '').toLowerCase();
                        if (this.stats.hasOwnProperty(status)) {
                            this.stats[status]++;
                        }
                    }
                }
            }
        },

        async updateDiscoveryStatus(job) {
            try {
                const response = await fetch(
                    `/api/v1/discovery/status?renovate=${encodeURIComponent(job.name)}&namespace=${encodeURIComponent(job.namespace)}`
                );
                if (response.ok) {
                    const data = await response.json();
                    job.discoveryRunning = data.status === 'running';
                } else {
                    job.discoveryRunning = false;
                }
            } catch (err) {
                job.discoveryRunning = false;
            }
        },

        async runDiscovery(job) {
            if (job.discoveryRunning) return;

            job.discoveryRunning = true;

            try {
                const response = await fetch('/api/v1/discovery/start', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        renovateJob: job.name,
                        namespace: job.namespace,
                    })
                });

                if (response.ok) {
                    Alpine.store('toastStore').success(
                        'Discovery Started',
                        `Discovery job for ${job.name} has been triggered`
                    );
                    setTimeout(() => this.loadJobs(), 1000);
                } else {
                    const errorText = await response.text();
                    throw new Error(errorText || 'Failed to start discovery');
                }
            } catch (err) {
                console.error('Error running discovery:', err);
                job.discoveryRunning = false;
                Alpine.store('toastStore').error('Discovery Failed', err.message);
            }
        },

        async triggerRenovate(job, project) {
            if (project.triggering) return;

            project.triggering = true;

            try {
                const response = await fetch('/api/v1/renovate', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        renovateJob: job.name,
                        namespace: job.namespace,
                        project: project.name
                    })
                });

                if (response.ok) {
                    Alpine.store('toastStore').success(
                        'Renovate Triggered',
                        `Job triggered for ${project.name}`
                    );
                    setTimeout(() => this.loadJobs(), 1000);
                } else {
                    const errorText = await response.text();
                    throw new Error(errorText || 'Failed to trigger renovate');
                }
            } catch (err) {
                console.error('Error triggering renovate:', err);
                Alpine.store('toastStore').error('Trigger Failed', err.message);
            } finally {
                project.triggering = false;
            }
        }
    });

    Alpine.store('dashboard').init();
});
