// jsdom doesn't implement window.scrollTo / Element.scrollIntoView; stub them
// so DocPage's scroll-to-anchor effect doesn't spam test output.
window.scrollTo = () => {};
if (!window.Element.prototype.scrollIntoView) {
  window.Element.prototype.scrollIntoView = () => {};
}
