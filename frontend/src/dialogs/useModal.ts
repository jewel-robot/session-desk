import { onMounted, ref, watch } from 'vue'

// Keeps a native <dialog> in sync with a boolean flag in the store.
export function useModal(isOpen: () => boolean, setOpen: (value: boolean) => void) {
  const element = ref<HTMLDialogElement>()
  const sync = (open: boolean) => {
    const dialog = element.value
    if (!dialog) return
    if (open && !dialog.open) dialog.showModal()
    if (!open && dialog.open) dialog.close()
  }
  watch(isOpen, sync, { flush: 'post' })
  onMounted(() => sync(isOpen()))
  const onClose = () => setOpen(false)
  return { element, onClose }
}
