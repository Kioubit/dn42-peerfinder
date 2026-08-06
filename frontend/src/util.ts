export async function copyText(text: string, e: MouseEvent, activeClass = '', inactiveClass = '') {
    const btn = e.currentTarget as HTMLButtonElement;
    try {
        const toggleElems = btn.querySelectorAll("[data-copy-toggle]")
        const toggleHidden = () => {
            toggleElems.forEach(el => {
                if (el.hasAttribute("hidden")) {
                    el.removeAttribute("hidden");
                } else {
                    el.setAttribute("hidden", "");
                }
            })
        }

        await navigator.clipboard.writeText(text);

        if (toggleElems.length === 0) return;

        btn.disabled = true;
        if (activeClass && inactiveClass) {
            btn.classList.add(activeClass);
            btn.classList.remove(inactiveClass);
        }

        toggleHidden();

        setTimeout(() => {
            btn.disabled = false;
            if (activeClass && inactiveClass) {
                btn.classList.add(inactiveClass);
                btn.classList.remove(activeClass);
            }
            toggleHidden();
        }, 2000);
    } catch (err) {
        console.warn(`Failed to copy text to clipboard: ${err}`);
    }
}