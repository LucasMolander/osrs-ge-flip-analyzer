const { createApp, ref, onMounted } = Vue

createApp({
    setup() {
        const items = ref([])
        const loading = ref(false)
        const submitting = ref(false)
        const error = ref(null)

        // Modal states
        const showFlipModal = ref(false)
        const showFailedModal = ref(false)
        const selectedItem = ref(null)

        const flipForm = ref({
            quantity: 1,
            buy_price: null,
            sell_price: null,
            note: ''
        })

        const failedForm = ref({
            target_qty: null,
            bought_qty: 0,
            buy_price: null,
            time_spent: '1h',
            note: ''
        })

        const formatNumber = (num) => {
            if (num === undefined || num === null) return '';
            if (num >= 1_000_000_000) return (num / 1_000_000_000).toFixed(1) + 'B';
            if (num >= 1_000_000) return (num / 1_000_000).toFixed(1) + 'M';
            if (num >= 1_000) return (num / 1_000).toFixed(1) + 'K';
            return num.toString();
        }

        const fetchReport = async () => {
            loading.value = true
            error.value = null
            try {
                const response = await fetch('/api/report')
                if (!response.ok) throw new Error('Failed to fetch market report')
                const data = await response.json()
                items.value = data || []
            } catch (err) {
                error.value = err.message
                console.error(err)
            } finally {
                loading.value = false
            }
        }

        const recordFlip = (item) => {
            selectedItem.value = item
            flipForm.value = {
                quantity: Math.min(item.buy_limit, 100),
                buy_price: item.low_mod,
                sell_price: item.high_mod,
                note: ''
            }
            showFlipModal.value = true
        }

        const recordFailedBuy = (item) => {
            selectedItem.value = item
            failedForm.value = {
                target_qty: Math.min(item.buy_limit, 100),
                bought_qty: 0,
                buy_price: item.low_mod,
                time_spent: '1h',
                note: ''
            }
            showFailedModal.value = true
        }

        const closeModals = () => {
            showFlipModal.value = false
            showFailedModal.value = false
            selectedItem.value = null
        }

        const submitFlip = async () => {
            submitting.value = true
            try {
                const payload = {
                    item_id: selectedItem.value.item_id,
                    quantity: flipForm.value.quantity,
                    buy_price: flipForm.value.buy_price,
                    sell_price: flipForm.value.sell_price,
                    note: flipForm.value.note
                }
                const response = await fetch('/api/flips', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                })
                if (!response.ok) throw new Error('Failed to record flip')
                
                closeModals()
                // Refresh data to see the new score
                fetchReport()
            } catch (err) {
                alert(err.message)
            } finally {
                submitting.value = false
            }
        }

        const submitFailedBuy = async () => {
            submitting.value = true
            try {
                const payload = {
                    item_id: selectedItem.value.item_id,
                    item_name: selectedItem.value.name,
                    target_qty: failedForm.value.target_qty,
                    bought_qty: failedForm.value.bought_qty,
                    buy_price: failedForm.value.buy_price,
                    time_spent: failedForm.value.time_spent,
                    report_score: selectedItem.value.score,
                    note: failedForm.value.note
                }
                const response = await fetch('/api/failed-buys', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload)
                })
                if (!response.ok) throw new Error('Failed to record failure')
                
                closeModals()
                // Refresh data to see the penalized score
                fetchReport()
            } catch (err) {
                alert(err.message)
            } finally {
                submitting.value = false
            }
        }

        onMounted(() => {
            fetchReport()
        })

        return {
            items,
            loading,
            submitting,
            error,
            showFlipModal,
            showFailedModal,
            selectedItem,
            flipForm,
            failedForm,
            formatNumber,
            fetchReport,
            recordFlip,
            recordFailedBuy,
            closeModals,
            submitFlip,
            submitFailedBuy
        }
    }
}).mount('#app')
