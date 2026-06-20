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
        const currentTab = ref('report') // 'report', 'manual', 'history', 'settings'
        
        const items = ref([])
        const itemDict = ref([]) // For autocomplete
        const flipsHistory = ref([])
        const failedBuysHistory = ref([])

        const loading = ref(false)
        const submitting = ref(false)
        const error = ref(null)
        const errorStack = ref(null)
        const showStack = ref(false)
        const success = ref(null)

        // Theme state
        const isDarkMode = ref(true)
        
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

        const formatSpike = (ind) => {
            return ind.replace('⚠️', '').replace('-Spike', '');
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
                const response = await fetchWithAuth('/api/report')
                items.value = await response.json() || []
            } catch (err) {
                if (err.message !== "unauthorized") {
                    showError(err.message, err.stackTrace)
                }
            } finally {
                loading.value = false
            }
        }

        const fetchHistory = async () => {
            if (!isAuthenticated.value) return
            loading.value = true
            try {
                const [resFlips, resFailed] = await Promise.all([
                    fetchWithAuth('/api/history/flips'),
                    fetchWithAuth('/api/history/failed-buys')
                ])
                flipsHistory.value = await resFlips.json() || []
                failedBuysHistory.value = await resFailed.json() || []
            } catch (err) {
                if (err.message !== "unauthorized") {
                    showError(err.message, err.stackTrace)
                }
            } finally {
                loading.value = false
            }
        }

        const syncPrices = async () => {
            loading.value = true
            try {
                await fetchWithAuth('/api/sync/prices', { method: 'POST' })
                showSuccess('Latest prices and volumes synchronized successfully!')
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
            flipForm.value = { quantity: Math.min(item.buy_limit, 100), buy_price: item.low_mod, sell_price: item.high_mod, note: '' }
            showFlipModal.value = true
        }

        const recordFailedBuy = (item) => {
            selectedItem.value = item
            failedForm.value = { target_qty: Math.min(item.buy_limit, 100), bought_qty: 0, buy_price: item.low_mod, time_spent: '1h', note: '' }
            showFailedModal.value = true
        }

        const closeModals = () => { showFlipModal.value = false; showFailedModal.value = false; selectedItem.value = null }

        const submitFlipPayload = async (payload) => {
            submitting.value = true
            try {
                await fetchWithAuth('/api/flips', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                })
                showSuccess('Flip recorded successfully!')
            } catch (err) {
                if (err.message !== "unauthorized") showError(err.message, err.stackTrace)
            } finally {
                submitting.value = false
            }
        }

        const submitFailedBuyPayload = async (payload) => {
            submitting.value = true
            try {
                await fetchWithAuth('/api/failed-buys', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                })
                showSuccess('Failed buy recorded successfully!')
            } catch (err) {
                if (err.message !== "unauthorized") showError(err.message, err.stackTrace)
            } finally {
                submitting.value = false
            }
        }

        const submitFlip = async () => {
            await submitFlipPayload({
                item_id: selectedItem.value.item_id,
                quantity: flipForm.value.quantity,
                buy_price: flipForm.value.buy_price,
                sell_price: flipForm.value.sell_price,
                note: flipForm.value.note
            })
            closeModals()
            fetchReport()
        }

        const submitFailedBuy = async () => {
            await submitFailedBuyPayload({
                item_id: selectedItem.value.item_id,
                item_name: selectedItem.value.name,
                target_qty: failedForm.value.target_qty,
                bought_qty: failedForm.value.bought_qty,
                buy_price: failedForm.value.buy_price,
                time_spent: failedForm.value.time_spent,
                report_score: selectedItem.value.score,
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
                quantity: manualFlipForm.value.quantity,
                buy_price: manualFlipForm.value.buy_price,
                sell_price: manualFlipForm.value.sell_price,
                note: manualFlipForm.value.note
            })
            manualFlipForm.value = { item: null, quantity: 1, buy_price: null, sell_price: null, note: '' }
        }

        const submitManualFailedBuy = async () => {
            if (!manualFailedForm.value.item) return
            await submitFailedBuyPayload({
                item_id: manualFailedForm.value.item.id,
                item_name: manualFailedForm.value.item.name,
                target_qty: manualFailedForm.value.target_qty,
                bought_qty: manualFailedForm.value.bought_qty,
                buy_price: manualFailedForm.value.buy_price,
                time_spent: manualFailedForm.value.time_spent,
                report_score: 0, // No specific score context
                note: manualFailedForm.value.note
            })
            manualFailedForm.value = { item: null, target_qty: null, bought_qty: 0, buy_price: null, time_spent: '1h', note: '' }
        }

        const toggleTheme = () => {
            isDarkMode.value = !isDarkMode.value
            document.documentElement.setAttribute('data-theme', isDarkMode.value ? 'dark' : 'light')
            localStorage.setItem('ge_analyzer_theme', isDarkMode.value ? 'dark' : 'light')
        }

        // Tab watcher
        Vue.watch(currentTab, (newTab) => {
            if (newTab === 'report') fetchReport()
            if (newTab === 'history') fetchHistory()
        })

        onMounted(() => {
            // Initialize theme
            const savedTheme = localStorage.getItem('ge_analyzer_theme')
            if (savedTheme === 'light') {
                isDarkMode.value = false
                document.documentElement.setAttribute('data-theme', 'light')
            }

            checkAuth()
            if (isAuthenticated.value) {
                fetchItemDict()
                fetchReport()
            }
        })

        return {
            currentTab, items, itemDict, flipsHistory, failedBuysHistory,
            loading, submitting, error, errorStack, showStack, success, selectedFile,
            isAuthenticated, loginForm, login, logout,
            isDarkMode, toggleTheme,
            showFlipModal, showFailedModal, selectedItem,
            flipForm, failedForm, manualFlipForm, manualFailedForm,
            formatNumber, getGoldColorClass, getWikiLink, formatSpike, formatDate, fetchReport, fetchHistory, syncPrices, syncMetadata,
            handleFileUpload, restoreBackup, recordFlip, recordFailedBuy, closeModals,
            submitFlip, submitFailedBuy, submitManualFlip, submitManualFailedBuy,
            clearError
        }
    }
}).mount('#app')
