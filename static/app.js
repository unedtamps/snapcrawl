// SnapCrawl AI — Alpine.js Application

function appData() {
    return {
        // ── State ──
        projects: [],
        currentProjectID: null,
        currentProject: null,
        newProjectName: '',
        activeTab: 'config',
        showParams: false,

        config: {
            baseUrl: '',
            schema: '',
            prompt: '',
            provider: 'deepseek',
            cookies: '',
        },

        urlParams: [{ key: '', value: '', enabled: true, mode: 'static', type: 'query' }],

        extractionPrompt: '',
        extractionConfig: '',
        extractionTemperature: 0.2,
        extractionMaxTokens: 2000,
        generatingExtractionConfig: false,
        testingExtraction: false,
        extractionTestResult: '',
        extractionTestError: '',
        extractionTestPreview: '',

        apiConfig: {
            enabled: false,
            params: [{ name: '', type: 'string', required: false, default_value: '', description: '' }],
        },

        // ── Settings & LLM Providers ──
        showSettingsModal: false,
        llmProviders: [],
        newProvider: { name: '', base_url: '', model_name: '', api_key: '', provider_type: 'cloud' },

        modal: { show: false, content: '' },
        toasts: [],
        toastCounter: 0,

        latestResult: null,

        // ── Batch Iteration State ──
        batchConfig: {},
        batchState: { running: false, progress: 0, total: 0, results: [], combinations: [] },
        batchController: null,

        // ── Lifecycle ──
        async init() {
            await this.loadLLMProviders();
            await this.loadProjects();

            // Close modal on Escape
            document.addEventListener('keydown', (e) => {
                if (e.key === 'Escape') this.closeModal();
            });
        },

        // ── LLM Providers ──
        async loadLLMProviders() {
            try {
                const res = await fetch('/api/providers');
                if (res.ok) {
                    this.llmProviders = (await res.json()) || [];
                    // Ensure a default is selected if config is empty or invalid
                    if (this.llmProviders.length > 0 && !this.llmProviders.find(p => p.id.toString() === this.config.provider)) {
                        this.config.provider = this.llmProviders[0].id.toString();
                    } else if (this.llmProviders.length === 0) {
                        this.config.provider = '';
                    }
                }
            } catch (e) {
                console.error('Failed to load providers:', e);
            }
        },

        async createLLMProvider() {
            try {
                const res = await fetch('/api/providers', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(this.newProvider),
                });
                if (res.ok) {
                    this.showToast('Provider added', 'success');
                    this.newProvider = { name: '', base_url: '', model_name: '', api_key: '', provider_type: 'cloud' };
                    await this.loadLLMProviders();
                } else {
                    const err = await res.json();
                    this.showToast(err.error || 'Failed to add provider', 'error');
                }
            } catch (e) {
                this.showToast('Error: ' + e.message, 'error');
            }
        },

        async deleteLLMProvider(id) {
            if (!confirm('Delete this provider?')) return;
            try {
                const res = await fetch('/api/providers/' + id, { method: 'DELETE' });
                if (res.ok) {
                    this.showToast('Provider deleted', 'success');
                    await this.loadLLMProviders();
                } else {
                    const err = await res.json();
                    this.showToast(err.error || 'Failed to delete provider', 'error');
                }
            } catch (e) {
                this.showToast('Error: ' + e.message, 'error');
            }
        },

        // ── Projects ──
        async loadProjects() {
            try {
                const res = await fetch('/projects');
                if (res.ok) this.projects = await res.json();
            } catch (e) {
                console.error('Failed to load projects:', e);
            }
        },

        async createProject() {
            const name = this.newProjectName.trim();
            if (!name) return;

            try {
                const res = await fetch('/projects', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ name }),
                });

                if (res.ok) {
                    const project = await res.json();
                    this.newProjectName = '';
                    this.showToast(`Project "${name}" created`, 'success');
                    await this.loadProjects();
                    this.selectProject(project.id);
                } else {
                    const err = await res.json();
                    this.showToast(err.error || 'Failed to create project', 'error');
                }
            } catch (e) {
                this.showToast('Error: ' + e.message, 'error');
            }
        },

        async selectProject(id) {
            this.currentProjectID = id;
            try {
                const res = await fetch(`/projects/${id}`);
                if (res.ok) {
                    this.currentProject = await res.json();
                    this.activeTab = 'config';
                    this.loadProjectData();
                }
            } catch (e) {
                console.error('Failed to load project:', e);
            }
        },

        loadProjectData() {
            if (!this.currentProject) return;

            this.config.baseUrl = this.currentProject.base_url || '';
            this.config.schema = this.currentProject.schema || '';
            this.config.prompt = this.currentProject.prompt || '';
            this.config.provider = this.currentProject.provider || 'deepseek';
            this.config.cookies = this.currentProject.cookies || '';

            this.extractionPrompt = this.currentProject.prompt || '';
            this.extractionConfig = this.currentProject.extraction_config || '';
            this.extractionTemperature = 0.2;
            this.extractionMaxTokens = 2000;
            this.generatingExtractionConfig = false;
            this.testingExtraction = false;
            this.extractionTestResult = '';
            this.extractionTestError = '';
            this.extractionTestPreview = '';

            this.parseUrlToParams();
            this.loadAPIConfig();
            this.loadLatestData();
        },

        async deleteProject() {
            if (!this.currentProjectID) return;
            if (!confirm('Delete this project and all its data?')) return;

            try {
                const res = await fetch(`/projects/${this.currentProjectID}`, { method: 'DELETE' });
                if (res.ok) {
                    this.showToast('Project deleted', 'success');
                    this.currentProjectID = null;
                    this.currentProject = null;
                    this.loadProjects();
                }
            } catch (e) {
                this.showToast('Error: ' + e.message, 'error');
            }
        },

        // ── URL Parameters ──
        parseUrlToParams() {
            const urlString = this.config.baseUrl.trim();
            if (!urlString) {
                this.urlParams = [{ key: '', value: '', enabled: true, mode: 'static', type: 'query' }];
                return;
            }

            try {
                const validUrl = urlString.startsWith('http') ? urlString : `https://${urlString}`;
                const urlObj = new URL(validUrl);

                const newParams = [];

                // Parse path segments (skip empty first segment from leading /)
                const pathParts = urlObj.pathname.split('/').filter(Boolean);
                pathParts.forEach((segment, i) => {
                    const existing = this.urlParams.find(p => p.key === `path_${i + 1}` && p.type === 'path');
                    const mode = existing ? existing.mode : 'static';
                    newParams.push({
                        key: `path_${i + 1}`,
                        value: segment,
                        enabled: true,
                        mode,
                        type: 'path',
                    });
                });

                // Parse query parameters
                urlObj.searchParams.forEach((value, key) => {
                    const existing = this.urlParams.find(p => p.key === key && p.type === 'query');
                    let mode = existing ? existing.mode : 'static';
                    if (this.apiConfig.params.some(ap => ap.name === key)) {
                        mode = 'dynamic';
                    }
                    newParams.push({ key, value, enabled: true, mode, type: 'query' });
                });

                newParams.push({ key: '', value: '', enabled: true, mode: 'static', type: 'query' });
                this.urlParams = newParams;
            } catch (e) {
                // Invalid URL – do nothing
            }
        },

        updateUrlParam(index, field, value) {
            this.urlParams[index][field] = value;

            // Auto-add row if typing in the last row
            if (index === this.urlParams.length - 1 && (this.urlParams[index].key || this.urlParams[index].value)) {
                this.urlParams.push({ key: '', value: '', enabled: true, mode: 'static', type: 'query' });
            }

            this.buildUrlFromParams();
        },

        addUrlParam() {
            this.urlParams.push({ key: '', value: '', enabled: true, mode: 'static', type: 'query' });
        },

        removeUrlParam(index) {
            this.urlParams.splice(index, 1);
            if (this.urlParams.length === 0) {
                this.urlParams.push({ key: '', value: '', enabled: true, mode: 'static', type: 'query' });
            }
            this.buildUrlFromParams();
        },

        buildUrlFromParams() {
            const urlString = this.config.baseUrl.trim();
            if (!urlString) return;

            try {
                const validUrl = urlString.startsWith('http') ? urlString : `https://${urlString}`;
                const urlObj = new URL(validUrl);

                // Rebuild path from path params — always use actual values
                const pathParams = this.urlParams.filter(p => p.type === 'path' && p.enabled && p.key);
                pathParams.sort((a, b) => {
                    const aNum = parseInt(a.key.replace('path_', '')) || 0;
                    const bNum = parseInt(b.key.replace('path_', '')) || 0;
                    return aNum - bNum;
                });

                const pathSegments = pathParams.map(p => p.value).filter(Boolean);
                urlObj.pathname = '/' + pathSegments.join('/');

                // Rebuild query string from query params
                urlObj.search = '';
                this.urlParams.forEach(p => {
                    if (p.type === 'query' && p.enabled && p.key) {
                        urlObj.searchParams.append(p.key, p.value);
                    }
                });

                this.config.baseUrl = urlObj.toString();
            } catch (e) {}
        },

        // ── Config Save ──
        async saveConfig() {
            if (!this.currentProjectID) return;

            if (!this.config.baseUrl.trim()) {
                this.showToast('Base URL is required', 'error');
                return;
            }

            if (this.extractionConfig.trim()) {
                try {
                    JSON.parse(this.extractionConfig.trim());
                } catch (e) {
                    this.showToast('Invalid extraction config JSON: ' + e.message, 'error');
                    return;
                }
            }

            // Build the save URL with placeholders for dynamic path params
            let saveUrl = this.config.baseUrl.trim();
            try {
                const urlObj = new URL(saveUrl.startsWith('http') ? saveUrl : 'https://' + saveUrl);
                const pathParams = this.urlParams.filter(p => p.type === 'path' && p.enabled && p.key);
                pathParams.sort((a, b) => {
                    const aNum = parseInt(a.key.replace('path_', '')) || 0;
                    const bNum = parseInt(b.key.replace('path_', '')) || 0;
                    return aNum - bNum;
                });
                const segments = pathParams.map(p => {
                    if (p.mode === 'dynamic') return '{' + p.key + '}';
                    return p.value;
                }).filter(Boolean);
                urlObj.pathname = '/' + segments.join('/');
                saveUrl = urlObj.toString();
            } catch (e) {}

            try {
                const res = await fetch(`/projects/${this.currentProjectID}`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        name: this.currentProject.name,
                        base_url: saveUrl,
                        schema: this.config.schema || '{}',
                        prompt: this.extractionPrompt.trim(),
                        provider: this.config.provider,
                        extraction_config: this.extractionConfig.trim(),
                        cookies: this.config.cookies.trim(),
                    }),
                });

                if (res.ok) {
                    this.showToast('Configuration saved!', 'success');

                    // Auto-sync dynamic params: merge into existing API params without overwriting user changes
                    const dynamicParams = this.urlParams
                        .filter(p => p.key.trim() && p.enabled && p.mode === 'dynamic');

                    if (dynamicParams.length > 0) {
                        // Build a map of existing API params by name to preserve user edits (type, required, description)
                        const existingByName = {};
                        for (const ep of this.apiConfig.params) {
                            if (ep.name) existingByName[ep.name] = ep;
                        }

                        const mergedParams = dynamicParams.map(p => {
                            const existing = existingByName[p.key];
                            if (existing) {
                                // Preserve user-edited fields, only update default_value from URL
                                return {
                                    ...existing,
                                    default_value: p.value,
                                };
                            }
                            // New param: auto-detect type from value
                            const detectedType = (p.value !== '' && !isNaN(Number(p.value))) ? 'number' : 'string';
                            return {
                                name: p.key,
                                type: detectedType,
                                required: false,
                                default_value: p.value,
                                description: '',
                            };
                        });

                        await fetch(`/projects/${this.currentProjectID}/api-config`, {
                            method: 'PUT',
                            headers: { 'Content-Type': 'application/json' },
                            body: JSON.stringify({
                                enabled: this.apiConfig.enabled,
                                params: mergedParams,
                            }),
                        });
                        this.showToast(`${mergedParams.length} dynamic param(s) synced to Interface`, 'info');
                    }

                    this.loadAPIConfig();
                } else {
                    const err = await res.json();
                    this.showToast('Error: ' + (err.error || 'Unknown error'), 'error');
                }
            } catch (e) {
                this.showToast('Error: ' + e.message, 'error');
            }
        },

        // ── Results ──
        async loadLatestData() {
            if (!this.currentProjectID) return;
            try {
                const res = await fetch(`/projects/${this.currentProjectID}/data`);
                if (res.ok) {
                    const data = await res.json();
                    this.latestResult = data && data.length > 0 ? data[0] : null;
                }
            } catch (e) {
                console.error('Failed to load data:', e);
            }
        },

        formatJSON(data) {
            try {
                const parsed = typeof data === 'string' ? JSON.parse(data) : data;
                return JSON.stringify(parsed, null, 2);
            } catch (e) {
                return String(data);
            }
        },

        exportJSON() {
            if (this.currentProjectID) window.location.href = `/projects/${this.currentProjectID}/data`;
        },

        exportCSV() {
            if (this.currentProjectID) window.location.href = `/projects/${this.currentProjectID}/data.csv`;
        },

        // ── Page Preview ──
        async previewPage() {
            const baseUrl = this.config.baseUrl.trim();
            if (!baseUrl) { this.showToast('Please enter a Base URL first', 'error'); return; }

            this.pagePreview.loading = true;
            this.pagePreview.error = '';
            this.pagePreview.markdown = '';
            this.pagePreview.size = '';

            try {
                const res = await fetch('/api/preview-markdown', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ url: baseUrl }),
                });

                const result = await res.json();

                if (res.ok) {
                    this.pagePreview.markdown = result.markdown;
                    this.pagePreview.size = result.size;
                    this.showToast('Page preview loaded', 'success');
                } else {
                    this.pagePreview.error = result.error || 'Failed to fetch page';
                    this.showToast('Error: ' + (result.error || 'Failed to fetch page'), 'error');
                }
            } catch (e) {
                this.pagePreview.error = e.message;
                this.showToast('Error: ' + e.message, 'error');
            } finally {
                this.pagePreview.loading = false;
            }
        },

        // ── Extraction Config ──
        async generateExtractionConfig() {
            const baseUrl = this.config.baseUrl.trim();
            if (!baseUrl) { this.showToast('Please enter a Base URL first', 'error'); return; }
            if (!this.extractionPrompt.trim()) { this.showToast('Please describe what data to extract', 'error'); return; }

            this.generatingExtractionConfig = true;
            this.extractionTestResult = '';
            this.extractionTestError = '';
            this.extractionTestPreview = '';

            try {
                const res = await fetch('/api/generate-extraction-config', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        url: baseUrl,
                        prompt: this.extractionPrompt.trim(),
                        provider: this.config.provider,
                        temperature: this.extractionTemperature,
                        max_tokens: this.extractionMaxTokens,
                    }),
                });

                const result = await res.json();

                if (res.ok && result.config) {
                    this.extractionConfig = JSON.stringify(result.config, null, 2);
                    this.showToast(`Extraction config generated! (${result.tokens_used} tokens)`, 'success');
                } else {
                    this.showToast('Error: ' + (result.error || 'Failed to generate config'), 'error');
                }
            } catch (e) {
                this.showToast('Error: ' + e.message, 'error');
            } finally {
                this.generatingExtractionConfig = false;
            }
        },

        async testExtractionConfig() {
            const baseUrl = this.config.baseUrl.trim();
            if (!baseUrl) { this.showToast('Please enter a Base URL first', 'error'); return; }
            if (!this.extractionConfig.trim()) { this.showToast('Please generate or enter an extraction config first', 'error'); return; }

            let config;
            try {
                config = JSON.parse(this.extractionConfig.trim());
            } catch (e) {
                this.showToast('Invalid extraction config JSON: ' + e.message, 'error');
                return;
            }

            this.testingExtraction = true;
            this.extractionTestResult = '';
            this.extractionTestError = '';
            this.extractionTestPreview = '';

            try {
                const res = await fetch('/api/test-extraction-config', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ url: baseUrl, config }),
                });

                const result = await res.json();

                if (res.ok) {
                    this.extractionTestResult = `${result.count} items extracted in ${result.duration_ms}ms`;
                    this.extractionTestPreview = JSON.stringify(result.data, null, 2);
                    this.showToast('Extraction test successful', 'success');
                } else {
                    this.extractionTestError = result.error || 'Extraction test failed';
                    this.showToast('Error: ' + (result.error || 'Extraction test failed'), 'error');
                }
            } catch (e) {
                this.extractionTestError = e.message;
                this.showToast('Error: ' + e.message, 'error');
            } finally {
                this.testingExtraction = false;
            }
        },

        // ── API Interface ──
        async loadAPIConfig() {
            if (!this.currentProjectID) return;

            try {
                const res = await fetch(`/projects/${this.currentProjectID}/api-config`);
                if (res.ok) {
                    const config = await res.json();
                    this.apiConfig.enabled = config.enabled;
                    this.apiConfig.params = config.params || [];
                    if (this.apiConfig.params.length === 0) {
                        this.apiConfig.params.push({ name: '', type: 'string', required: false, default_value: '', description: '' });
                    }
                    this.updateCurlPreview();

                    // Re-parse URL params to restore dynamic states
                    this.parseUrlToParams();
                    
                    // Update batch iterations based on allowed params
                    this.updateBatchCombinations();
                }
            } catch (e) {
                console.error('Failed to load API config:', e);
            }
        },

        async saveAPIConfig() {
            if (!this.currentProjectID) return;

            const validParams = this.apiConfig.params.filter(p => p.name.trim() !== '');
            const config = {
                enabled: this.apiConfig.enabled,
                params: validParams,
            };

            try {
                const res = await fetch(`/projects/${this.currentProjectID}/api-config`, {
                    method: 'PUT',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(config),
                });

                if (res.ok) {
                    this.showToast('API configuration saved!', 'success');
                } else {
                    const err = await res.json();
                    this.showToast('Error: ' + (err.error || 'Unknown error'), 'error');
                }
            } catch (e) {
                this.showToast('Error: ' + e.message, 'error');
            }
        },

        updateApiParam(index, field, value) {
            this.apiConfig.params[index][field] = value;

            // Auto-add new row only when typing in name on last row
            if (index === this.apiConfig.params.length - 1 && field === 'name' && value.trim() !== '') {
                this.apiConfig.params.push({ name: '', type: 'string', required: false, default_value: '', description: '' });
            }

            this.updateCurlPreview();
        },

        addApiParam() {
            this.apiConfig.params.push({ name: '', type: 'string', required: false, default_value: '', description: '' });
        },

        removeApiParam(index) {
            this.apiConfig.params.splice(index, 1);
            if (this.apiConfig.params.length === 0) {
                this.apiConfig.params.push({ name: '', type: 'string', required: false, default_value: '', description: '' });
            }
            this.updateCurlPreview();
            this.updateBatchCombinations();
        },

        hasBatchParams() {
            const hasApiParams = this.apiConfig.enabled && this.apiConfig.params.some(p => !!p.name);
            const hasDynamicPath = this.urlParams.some(p => p.type === 'path' && p.mode === 'dynamic');
            return hasApiParams || hasDynamicPath;
        },

        // ── Batch Iteration Logic ──
        syncBatchConfig() {
            this.apiConfig.params.forEach(p => {
                if (!p.name) return;
                if (!this.batchConfig[p.name]) {
                    this.batchConfig[p.name] = p.type === 'number' 
                        ? { start: 1, end: 1, step: 1 } 
                        : { list: '' };
                }
            });
            this.urlParams.filter(p => p.type === 'path' && p.mode === 'dynamic').forEach(p => {
                if (!this.batchConfig[p.key]) {
                    this.batchConfig[p.key] = { list: '' };
                }
            });
        },

        updateBatchCombinations() {
            this.syncBatchConfig();
            
            const variations = {};

            // API query params
            for (const p of this.apiConfig.params.filter(p => !!p.name)) {
                variations[p.name] = [];
                const config = this.batchConfig[p.name];
                if (!config) continue;
                
                if (p.type === 'number') {
                    const start = parseFloat(config.start);
                    const end = parseFloat(config.end);
                    const step = parseFloat(config.step);
                    if (!isNaN(start) && !isNaN(end) && !isNaN(step) && step > 0) {
                        for (let v = start; v <= end; v += step) {
                            variations[p.name].push(v);
                        }
                    }
                } else {
                    if (config.list && config.list.trim()) {
                        variations[p.name] = config.list.split(',').map(s => s.trim()).filter(s => s);
                    }
                }
                
                if (variations[p.name].length === 0) {
                    this.batchState.combinations = [];
                    this.batchState.total = 0;
                    return;
                }
            }

            // Dynamic path params
            for (const p of this.urlParams.filter(p => p.type === 'path' && p.mode === 'dynamic')) {
                variations[p.key] = [];
                const config = this.batchConfig[p.key];
                if (config && config.list && config.list.trim()) {
                    variations[p.key] = config.list.split(',').map(s => s.trim()).filter(s => s);
                }
                if (variations[p.key].length === 0) {
                    this.batchState.combinations = [];
                    this.batchState.total = 0;
                    return;
                }
            }
            
            const paramNames = Object.keys(variations);
            if (paramNames.length === 0) {
                this.batchState.combinations = [];
                this.batchState.total = 0;
                return;
            }
            
            const combos = [];
            const helper = (paramIdx, currentCombo) => {
                if (paramIdx === paramNames.length) {
                    combos.push({...currentCombo});
                    return;
                }
                const name = paramNames[paramIdx];
                for (const val of variations[name]) {
                    currentCombo[name] = val;
                    helper(paramIdx + 1, currentCombo);
                }
            };
            
            helper(0, {});
            this.batchState.combinations = combos;
            this.batchState.total = combos.length;
        },

        async runBatchScrape() {
            this.updateBatchCombinations();
            if (this.batchState.combinations.length === 0) return;
            
            this.batchState.running = true;
            this.batchState.progress = 0;
            this.batchState.results = [];
            
            if (this.batchController) this.batchController.abort();
            this.batchController = new AbortController();
            const signal = this.batchController.signal;
            
            const baseUrl = `${window.location.origin}/api/public/${this.currentProjectID}/scrape`;
            
            try {
                for (const combo of this.batchState.combinations) {
                    if (signal.aborted) break;
                    
                    const urlObj = new URL(baseUrl);
                    for (const [k, v] of Object.entries(combo)) {
                        urlObj.searchParams.append(k, v);
                    }
                    
                    const res = await fetch(urlObj.toString(), { signal });
                    const result = await res.json();
                    
                    if (res.ok && result.data) {
                        const dataItems = Array.isArray(result.data) ? result.data : [result.data];
                        for (const item of dataItems) {
                            this.batchState.results.push({
                                _url: result.url,
                                _params: combo,
                                ...item
                            });
                        }
                    } else if (!res.ok) {
                        this.showToast('Batch item error: ' + (result.error || 'Unknown error'), 'error');
                    }
                    
                    this.batchState.progress++;
                }
                if (!signal.aborted) {
                    this.showToast('Batch scraping completed successfully.', 'success');
                }
            } catch (e) {
                if (e.name !== 'AbortError') {
                    this.showToast('Batch error: ' + e.message, 'error');
                } else {
                    this.showToast('Batch scraping canceled.', 'info');
                }
            } finally {
                this.batchState.running = false;
                this.batchController = null;
            }
        },

        cancelBatchScrape() {
            if (this.batchController) this.batchController.abort();
        },

        downloadBatchJSON() {
            if (this.batchState.results.length === 0) return;
            const dataStr = "data:text/json;charset=utf-8," + encodeURIComponent(JSON.stringify(this.batchState.results, null, 2));
            const a = document.createElement('a');
            a.href = dataStr;
            a.download = `batch_results_${this.currentProjectID}.json`;
            a.click();
        },

        downloadBatchCSV() {
            if (this.batchState.results.length === 0) return;
            const rows = this.batchState.results;
            
            const headersSet = new Set(['_url']);
            rows.forEach(r => Object.keys(r._params || {}).forEach(k => headersSet.add('param_' + k)));
            rows.forEach(r => Object.keys(r).forEach(k => {
                if (k !== '_url' && k !== '_params') headersSet.add(k);
            }));
            const headers = Array.from(headersSet);
            
            const csvRows = [headers.join(',')];
            
            rows.forEach(row => {
                const values = headers.map(h => {
                    let val = '';
                    if (h === '_url') val = row._url;
                    else if (h.startsWith('param_')) {
                        const paramName = h.replace('param_', '');
                        val = row._params ? row._params[paramName] : '';
                    } else {
                        val = row[h];
                    }
                    const strVal = String(val === null || val === undefined ? '' : val).replace(/"/g, '""');
                    return `"${strVal}"`;
                });
                csvRows.push(values.join(','));
            });
            
            const dataStr = "data:text/csv;charset=utf-8," + encodeURIComponent(csvRows.join('\n'));
            const a = document.createElement('a');
            a.href = dataStr;
            a.download = `batch_results_${this.currentProjectID}.csv`;
            a.click();
        },

        // ── Computed-like ──
        get endpointUrl() {
            if (!this.currentProjectID) return '—';
            return `${window.location.origin}/api/public/${this.currentProjectID}/scrape`;
        },

        get curlPreview() {
            if (!this.currentProjectID) return 'curl "..."';

            let url = `${window.location.origin}/api/public/${this.currentProjectID}/scrape`;
            const validParams = this.apiConfig.params.filter(p => p.name.trim() !== '');

            if (validParams.length > 0) {
                const parts = validParams.map(p => {
                    const val = p.default_value || (p.type === 'number' ? '0' : '{value}');
                    return `${encodeURIComponent(p.name)}=${encodeURIComponent(val)}`;
                });
                url += '?' + parts.join('&');
            }

            return `curl "${url}"`;
        },

        // A no-op trigger method to force Alpine reactivity for the getter
        updateCurlPreview() {
            // Getters are automatically reactive in Alpine, but we keep
            // this method name for backward-compat with event handlers.
        },

        copyEndpointURL() {
            const url = this.endpointUrl;
            if (url === '—') return;
            navigator.clipboard.writeText(url).then(() => {
                this.showToast('Endpoint URL copied!', 'success');
            });
        },

        // ── Modal ──
        showModal(content) {
            this.modal = { show: true, content };
        },

        closeModal() {
            this.modal.show = false;
        },

        // ── Toast ──
        showToast(message, type = 'info') {
            this.toasts.push({ id: ++this.toastCounter, message, type });
        },

        // ── Utility ──
        escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        },
    };
}
