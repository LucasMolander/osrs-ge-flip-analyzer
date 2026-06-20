const { createApp, ref, computed, onMounted, onUnmounted } = Vue

// Custom Autocomplete Component
const AutocompleteInput = {
    props: ['modelValue', 'options', 'placeholder'],
    emits: ['update:modelValue'],
    template: `
        <div class="autocomplete-wrapper" ref="wrapper">
            <input type="text" 
                   v-model="searchQuery" 
                   @focus="isOpen = true"
                   @input="onInput"
                   :placeholder="placeholder"
                   class="autocomplete-input">
            <div v-if="isOpen && filteredOptions.length > 0" class="autocomplete-dropdown glass-panel">
                <div v-for="option in filteredOptions" 
                     :key="option.id" 
                     class="autocomplete-item"
                     @click="selectOption(option)">
                    {{ option.name }}
                </div>
            </div>
            <div v-if="isOpen && filteredOptions.length === 0 && searchQuery" class="autocomplete-dropdown glass-panel">
                <div class="autocomplete-empty">No items found</div>
            </div>
        </div>
    `,
    setup(props, { emit }) {
        const searchQuery = ref('')
        const isOpen = ref(false)
        const wrapper = ref(null)

        // Close dropdown when clicking outside
        const handleClickOutside = (event) => {
            if (wrapper.value && !wrapper.value.contains(event.target)) {
                isOpen.value = false
            }
        }

        onMounted(() => {
            document.addEventListener('mousedown', handleClickOutside)
            if (props.modelValue && props.modelValue.name) {
                searchQuery.value = props.modelValue.name
            }
        })

        onUnmounted(() => {
            document.removeEventListener('mousedown', handleClickOutside)
        })

        const onInput = () => {
            isOpen.value = true
            emit('update:modelValue', null) // clear selection if typing
        }

        const filteredOptions = computed(() => {
            if (!searchQuery.value) return props.options.slice(0, 50) // Return top 50 if empty
            const query = searchQuery.value.toLowerCase()
            return props.options.filter(opt => opt.name.toLowerCase().includes(query)).slice(0, 50)
        })

        const selectOption = (option) => {
            searchQuery.value = option.name
            isOpen.value = false
            emit('update:modelValue', option)
        }

        return {
            searchQuery,
            isOpen,
            wrapper,
            filteredOptions,
            onInput,
            selectOption
        }
    }
}

createApp({
    components: {
        'autocomplete-input': AutocompleteInput
    },
    setup() {
        const initialTab = window.location.hash.replace('#', '') || 'report'
        const currentTab = ref(initialTab) // 'report', 'manual', 'history', 'settings'
        
        const items = ref([])
        const itemDict = ref([]) // For autocomplete
        const localConfig = ref(null)
        const flipsHistory = ref(JSON.parse(localStorage.getItem('ge_analyzer_flips') || '[]'))
        const failedSellsHistory = ref(JSON.parse(localStorage.getItem('ge_analyzer_failed_sells') || '[]'))

        // Watchers to auto-save to localStorage
        Vue.watch(flipsHistory, (newVal) => {
            localStorage.setItem('ge_analyzer_flips', JSON.stringify(newVal))
        }, { deep: true })
        Vue.watch(failedSellsHistory, (newVal) => {
            localStorage.setItem('ge_analyzer_failed_sells', JSON.stringify(newVal))
        }, { deep: true })
        Vue.watch(localConfig, (newVal) => {
            if (newVal) {
                localStorage.setItem('ge_analyzer_config', JSON.stringify(newVal))
            }
        }, { deep: true })

        const loading = ref(false)
        const submitting = ref(false)
        const error = ref(null)
        const errorStack = ref(null)
        const showStack = ref(false)
        const success = ref(null)

        // Theme state
        const isDarkMode = ref(true)

        // Sorting state
        const sortConfig = ref({ key: null, direction: 'default' })

        const sortBy = (key) => {
            if (sortConfig.value.key === key) {
                if (sortConfig.value.direction === 'asc') {
                    sortConfig.value.direction = 'desc'
                } else if (sortConfig.value.direction === 'desc') {
                    sortConfig.value.direction = 'default'
                    sortConfig.value.key = null
                } else {
                    sortConfig.value.direction = 'asc'
                }
            } else {
                sortConfig.value.key = key
                sortConfig.value.direction = 'asc'
            }
        }

        const sortedItems = Vue.computed(() => {
            if (!sortConfig.value.key || sortConfig.value.direction === 'default') {
                return items.value
            }
            
            return [...items.value].sort((a, b) => {
                let valA = a[sortConfig.value.key]
                let valB = b[sortConfig.value.key]
                
                // Handle missing values
                if (valA === undefined || valA === null) valA = ''
                if (valB === undefined || valB === null) valB = ''

                // String comparison
                if (typeof valA === 'string' && typeof valB === 'string') {
                    return sortConfig.value.direction === 'asc' 
                        ? valA.localeCompare(valB) 
                        : valB.localeCompare(valA)
                }

                // Numeric comparison
                return sortConfig.value.direction === 'asc' 
                    ? valA - valB 
                    : valB - valA
            })
        })

        // Percentage computed properties for UI binding
        const targetRoiPercent = computed({
            get: () => localConfig.value ? Math.round(localConfig.value.target_roi * 100) : 0,
            set: (val) => { if (localConfig.value) localConfig.value.target_roi = val / 100.0; }
        })
        const volatilityThresholdPercentUI = computed({
            get: () => localConfig.value ? Math.round(localConfig.value.volatility_threshold_percent * 100) : 0,
            set: (val) => { if (localConfig.value) localConfig.value.volatility_threshold_percent = val / 100.0; }
        })
        const spreadJitterHighPercent = computed({
            get: () => localConfig.value ? Math.round(localConfig.value.spread_jitter_high_threshold * 100) : 0,
            set: (val) => { if (localConfig.value) localConfig.value.spread_jitter_high_threshold = val / 100.0; }
        })
        const spreadJitterLowPercent = computed({
            get: () => localConfig.value ? Math.round(localConfig.value.spread_jitter_low_threshold * 100) : 0,
            set: (val) => { if (localConfig.value) localConfig.value.spread_jitter_low_threshold = val / 100.0; }
        })
        
        // Auth state
        const isAuthenticated = ref(false)
        const loginForm = ref({ username: '', password: '' })
        const authKey = 'ge_analyzer_auth'

        // Check local storage for valid auth (24h expiry)
        const checkAuth = () => {
            const stored = localStorage.getItem(authKey)
            if (stored) {
                try {
                    const data = JSON.parse(stored)
                    if (data.expiry > Date.now() && data.token) {
                        isAuthenticated.value = true
                        return data.token
                    } else {
                        localStorage.removeItem(authKey) // Expired
                    }
                } catch (e) {
                    localStorage.removeItem(authKey)
                }
            }
            isAuthenticated.value = false
            return null
        }

        const login = () => {
            const token = btoa(`${loginForm.value.username}:${loginForm.value.password}`)
            const expiry = Date.now() + 24 * 60 * 60 * 1000 // 24 hours
            localStorage.setItem(authKey, JSON.stringify({ token, expiry }))
            isAuthenticated.value = true
            loginForm.value = { username: '', password: '' }
            
            // Reload initial data
            fetchItemDict()
            fetchReport()
        }

        const logout = () => {
            localStorage.removeItem(authKey)
            isAuthenticated.value = false
            items.value = []
            itemDict.value = []
        }

        // Settings / File upload state
        const selectedFile = ref(null)

        // Modal states (used in Market Report tab)
        const showFlipModal = ref(false)
        const showFailedModal = ref(false)
        const selectedItem = ref(null)

        const flipForm = ref({ quantity: 1, buy_price: null, sell_price: null, note: '' })
        const failedForm = ref({ target_qty: null, bought_qty: 0, buy_price: null, time_spent: '1h', note: '' })

        // Manual Entry states
        const manualFlipForm = ref({ item: null, quantity: 1, buy_price: null, sell_price: null, note: '' })
        const manualFailedForm = ref({ item: null, target_qty: null, bought_qty: 0, buy_price: null, time_spent: '1h', note: '' })

        const formatNumber = (num) => {
            if (num === undefined || num === null) return '';
            const absN = Math.abs(num);
            const sign = num < 0 ? '-' : '';
            if (absN >= 10_000_000) return sign + (absN / 1_000_000).toFixed(3) + 'M';
            if (absN >= 1_000_000) return sign + Math.round(absN / 1_000).toString() + 'K';
            if (absN >= 100_000) return sign + (absN / 1_000).toFixed(2) + 'K';
            return sign + absN.toString();
        }

        const getGoldColorClass = (val) => {
            if (val >= 10_000_000) return 'gold-high';
            if (val >= 100_000) return 'gold-med';
            return 'gold-low';
        }

        const getWikiLink = (name) => {
            if (!name) return '#';
            return 'https://oldschool.runescape.wiki/w/' + encodeURIComponent(name.replace(/ /g, '_'));
        }


        const formatDate = (dateString) => {
            if (!dateString) return '';
            const d = new Date(dateString)
            return d.toLocaleString()
        }

        const showError = (msg, stack = null) => { 
            error.value = msg; 
            errorStack.value = stack;
            showStack.value = false;
        }
        const clearError = () => { error.value = null; errorStack.value = null; showStack.value = false; }
        const showSuccess = (msg) => { success.value = msg; setTimeout(() => { success.value = null }, 5000) }

        // Wrapper for fetch to include Authorization and handle 401/JSON errors
        const fetchWithAuth = async (url, options = {}) => {
            const token = checkAuth()
            if (!token) {
                throw new Error("unauthorized")
            }

            const headers = {
                ...options.headers,
                'Authorization': `Basic ${token}`
            }

            const res = await fetch(url, { ...options, headers })
            
            if (res.status === 401) {
                logout()
                throw new Error("unauthorized")
            }

            if (!res.ok) {
                let errMsg = `HTTP Error ${res.status}`
                let errStack = null
                try {
                    const data = await res.json()
                    errMsg = data.error || errMsg
                    errStack = data.stack_trace || null
                } catch (e) {
                    // Not JSON, just use status
                }
                const err = new Error(errMsg)
                err.stackTrace = errStack
                throw err
            }
            
            return res
        }

        // API Calls
        const fetchItemDict = async () => {
            if (!isAuthenticated.value) return
            try {
                const response = await fetchWithAuth('/api/items')
                itemDict.value = await response.json()
            } catch (err) { 
                if (err.message !== "unauthorized") console.error(err) 
            }
        }

        const fetchReport = async () => {
            if (!isAuthenticated.value) return
            loading.value = true
            try {
                const payload = {
                    config: localConfig.value,
                    flips: flipsHistory.value,
                    failed_sells: failedSellsHistory.value
                }
                const response = await fetchWithAuth('/api/report', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                })
                items.value = await response.json() || []
            } catch (err) {
                if (err.message !== "unauthorized") {
                    showError(err.message, err.stackTrace)
                }
            } finally {
                loading.value = false
            }
        }

        const fetchDefaultConfig = async () => {
            try {
                const response = await fetchWithAuth('/api/config/default')
                return await response.json()
            } catch (err) {
                console.error("Failed to fetch default config", err)
                return null
            }
        }

        const resetToDefaults = async () => {
            const defaults = await fetchDefaultConfig()
            if (defaults) {
                localConfig.value = defaults
                showSuccess("Reset to system defaults!")
                if (currentTab.value === 'report') fetchReport()
            }
        }

        const exportUserFile = () => {
            const data = {
                config: localConfig.value,
                flips: flipsHistory.value,
                failed_sells: failedSellsHistory.value
            }
            const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' })
            const url = URL.createObjectURL(blob)
            const a = document.createElement('a')
            a.href = url
            a.download = `osrs_analyzer_profile_${Date.now()}.json`
            document.body.appendChild(a)
            a.click()
            document.body.removeChild(a)
            URL.revokeObjectURL(url)
        }

        const importUserFile = (event) => {
            const file = event.target.files[0]
            if (!file) return
            const reader = new FileReader()
            reader.onload = (e) => {
                try {
                    const data = JSON.parse(e.target.result)
                    if (data.config) localConfig.value = data.config
                    if (data.flips) flipsHistory.value = data.flips
                    if (data.failed_sells) failedSellsHistory.value = data.failed_sells
                    showSuccess("Profile imported successfully!")
                    if (currentTab.value === 'report') fetchReport()
                } catch (err) {
                    showError("Failed to parse import file", err.stack)
                }
            }
            reader.readAsText(file)
            event.target.value = '' // reset input
        }

        const fetchHistory = async () => {
            // No-op, it's already in reactive variables tied to localStorage
        }



        const syncPrices = async () => {
            loading.value = true
            try {
                await fetchWithAuth('/api/sync/prices', { method: 'POST' })
                if (currentTab.value === 'report') fetchReport()
            } catch (err) {
                if (err.message !== "unauthorized") showError(err.message, err.stackTrace)
            } finally {
                loading.value = false
            }
        }

        const syncMetadata = async () => {
            loading.value = true
            try {
                await fetchWithAuth('/api/sync/metadata', { method: 'POST' })
                showSuccess('Item metadata synchronized successfully!')
                fetchItemDict()
            } catch (err) {
                if (err.message !== "unauthorized") showError(err.message, err.stackTrace)
            } finally {
                loading.value = false
            }
        }

        const handleFileUpload = (event) => {
            selectedFile.value = event.target.files[0]
        }

        const restoreBackup = async () => {
            if (!selectedFile.value) return
            loading.value = true
            try {
                const formData = new FormData()
                formData.append('backup_file', selectedFile.value)
                await fetchWithAuth('/api/restore', {
                    method: 'POST',
                    body: formData
                })
                showSuccess('Database restored successfully!')
                selectedFile.value = null
                
                // Refresh data if needed
                fetchItemDict()
                fetchReport()
                fetchHistory()
            } catch (err) {
                if (err.message !== "unauthorized") showError(err.message, err.stackTrace)
            } finally {
                loading.value = false
            }
        }

        // Action Modals (from Market Report)
        const recordFlip = (item) => {
            selectedItem.value = item
            flipForm.value = { rating: 'Good', note: '' }
            showFlipModal.value = true
        }

        const recordFailedBuy = (item) => {
            selectedItem.value = item
            failedForm.value = { note: '' }
            showFailedModal.value = true
        }

        const closeModals = () => { showFlipModal.value = false; showFailedModal.value = false; selectedItem.value = null }

        const submitFlipPayload = async (payload) => {
            flipsHistory.value.unshift({
                item_id: payload.item_id,
                item_name: payload.item_name,
                rating: payload.rating,
                timestamp: new Date().toISOString(),
                notes: payload.note
            })
            showSuccess('Flip recorded successfully!')
        }

        const submitFailedBuyPayload = async (payload) => {
            failedSellsHistory.value.unshift({
                item_id: payload.item_id,
                item_name: payload.item_name,
                timestamp: new Date().toISOString(),
                notes: payload.note
            })
            showSuccess('Failed sell recorded successfully!')
        }

        const submitFlip = async () => {
            await submitFlipPayload({
                item_id: selectedItem.value.item_id,
                item_name: selectedItem.value.name,
                rating: flipForm.value.rating,
                note: flipForm.value.note
            })
            closeModals()
            fetchReport()
        }

        const submitFailedBuy = async () => {
            await submitFailedBuyPayload({
                item_id: selectedItem.value.item_id,
                item_name: selectedItem.value.name,
                note: failedForm.value.note
            })
            closeModals()
            fetchReport()
        }

        // Manual Entry Forms
        const submitManualFlip = async () => {
            if (!manualFlipForm.value.item) return
            await submitFlipPayload({
                item_id: manualFlipForm.value.item.id,
                item_name: manualFlipForm.value.item.name,
                rating: manualFlipForm.value.rating,
                note: manualFlipForm.value.note
            })
            manualFlipForm.value = { item: null, rating: 'Good', note: '' }
        }

        const submitManualFailedBuy = async () => {
            if (!manualFailedForm.value.item) return
            await submitFailedBuyPayload({
                item_id: manualFailedForm.value.item.id,
                item_name: manualFailedForm.value.item.name,
                note: manualFailedForm.value.note
            })
            manualFailedForm.value = { item: null, note: '' }
        }

        const toggleTheme = () => {
            isDarkMode.value = !isDarkMode.value
            document.documentElement.setAttribute('data-theme', isDarkMode.value ? 'dark' : 'light')
            localStorage.setItem('ge_analyzer_theme', isDarkMode.value ? 'dark' : 'light')
        }

        // Tab watcher
        Vue.watch(currentTab, (newTab) => {
            window.location.hash = newTab
            if (newTab === 'report') fetchReport()
            if (newTab === 'history') fetchHistory()
        })
        
        // Listen for hash changes
        window.addEventListener('hashchange', () => {
            const hashTab = window.location.hash.replace('#', '')
            if (['report', 'manual', 'history', 'settings'].includes(hashTab)) {
                currentTab.value = hashTab
            }
        })

        onMounted(async () => {
            // Initialize theme
            const savedTheme = localStorage.getItem('ge_analyzer_theme')
            if (savedTheme === 'light') {
                isDarkMode.value = false
                document.documentElement.setAttribute('data-theme', 'light')
            }

            checkAuth()
            if (isAuthenticated.value) {
                // Initialize local config by merging saved values with current defaults
                const defaults = await fetchDefaultConfig()
                const storedConfig = localStorage.getItem('ge_analyzer_config')
                if (storedConfig) {
                    try {
                        const parsed = JSON.parse(storedConfig)
                        localConfig.value = { ...defaults, ...parsed }
                    } catch (e) {
                        localConfig.value = defaults
                    }
                } else {
                    localConfig.value = defaults
                }

                fetchItemDict()
                fetchReport()
            }
        })

        return {
            currentTab, items, itemDict, flipsHistory, failedSellsHistory, localConfig,
            sortedItems, sortConfig, sortBy,
            loading, submitting, error, errorStack, showStack, success, selectedFile,
            isAuthenticated, loginForm, login, logout,
            isDarkMode, toggleTheme,
            showFlipModal, showFailedModal, selectedItem,
            flipForm, failedForm, manualFlipForm, manualFailedForm,
            formatNumber, getGoldColorClass, getWikiLink, formatDate, fetchReport, fetchHistory, syncPrices, syncMetadata,
            handleFileUpload, restoreBackup, recordFlip, recordFailedBuy, closeModals,
            submitFlip, submitFailedBuy, submitManualFlip, submitManualFailedBuy,
            resetToDefaults, exportUserFile, importUserFile,
            clearError,
            targetRoiPercent, volatilityThresholdPercentUI, spreadJitterHighPercent, spreadJitterLowPercent
        }
    }
}).mount('#app')
