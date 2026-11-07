// Copy is a progressive enhancement: the readonly text remains usable without JS.
(() => {
  document.querySelectorAll('[data-copy-target]').forEach((button) => {
    button.hidden = false;
  });

  document.addEventListener('click', async (event) => {
    if (!(event.target instanceof Element)) return;
    const button = event.target.closest('[data-copy-target]');
    if (!(button instanceof HTMLButtonElement) || button.disabled) return;
    const target = document.getElementById(button.dataset.copyTarget);
    const status = document.getElementById(button.dataset.copyStatus);
    if (!(target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement) || !status) return;
    button.disabled = true;
    status.textContent = '';
    try {
      if (!navigator.clipboard?.writeText) throw new Error('Clipboard unavailable');
      await navigator.clipboard.writeText(target.value);
      status.textContent = `${button.dataset.copyLabel}をコピーしました。`;
    } catch {
      target.focus();
      target.select();
      target.setSelectionRange(0, target.value.length);
      status.textContent = '選択したテキストを手動でコピーしてください。';
    } finally {
      button.disabled = false;
    }
  });
})();
