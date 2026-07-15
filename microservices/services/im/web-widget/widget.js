/*!
 * Go Admin Kit IM Widget (M2)
 * Usage:
 * <script src="https://your-gateway/im/widget/widget.js"
 *   data-app-key="demo"
 *   data-base=""  optional, default = script origin
 *   async></script>
 */
(function () {
  'use strict';

  var SCRIPT = document.currentScript;
  if (!SCRIPT) {
    var scripts = document.getElementsByTagName('script');
    SCRIPT = scripts[scripts.length - 1];
  }

  var APP_KEY = (SCRIPT && SCRIPT.getAttribute('data-app-key')) || 'demo';
  var BASE = (SCRIPT && SCRIPT.getAttribute('data-base')) || '';
  if (!BASE && SCRIPT && SCRIPT.src) {
    try {
      var u = new URL(SCRIPT.src);
      BASE = u.origin;
    } catch (e) {
      BASE = '';
    }
  }

  var BTN_ID = 'goadmin-im-launcher';
  var FRAME_ID = 'goadmin-im-frame';
  var open = false;

  function ensureStyles() {
    if (document.getElementById('goadmin-im-style')) return;
    var css = document.createElement('style');
    css.id = 'goadmin-im-style';
    css.textContent =
      '#' + BTN_ID + '{position:fixed;right:20px;bottom:20px;z-index:2147483000;' +
      'width:56px;height:56px;border-radius:50%;border:0;cursor:pointer;' +
      'background:linear-gradient(135deg,#4f46e5,#7c3aed);color:#fff;font-size:22px;' +
      'box-shadow:0 10px 30px rgba(79,70,229,.45);display:flex;align-items:center;justify-content:center;}' +
      '#' + BTN_ID + ':hover{transform:scale(1.05);}' +
      '#' + FRAME_ID + '{position:fixed;right:20px;bottom:88px;z-index:2147483000;' +
      'width:380px;height:560px;max-width:calc(100vw - 24px);max-height:calc(100vh - 100px);' +
      'border:0;border-radius:16px;box-shadow:0 18px 50px rgba(15,23,42,.25);display:none;background:#fff;}' +
      '@media (max-width:480px){#' + FRAME_ID + '{right:0;bottom:0;width:100vw;height:100vh;border-radius:0;max-height:none;}}';
    document.head.appendChild(css);
  }

  function frameSrc() {
    var parentOrigin = encodeURIComponent(location.origin);
    var q = 'app_key=' + encodeURIComponent(APP_KEY) + '&parent_origin=' + parentOrigin;
    return (BASE || '') + '/im/widget/frame.html?' + q;
  }

  function ensureFrame() {
    var frame = document.getElementById(FRAME_ID);
    if (frame) return frame;
    frame = document.createElement('iframe');
    frame.id = FRAME_ID;
    frame.title = '在线客服';
    frame.allow = 'clipboard-write';
    frame.src = frameSrc();
    document.body.appendChild(frame);
    return frame;
  }

  function setOpen(next) {
    open = next;
    var frame = ensureFrame();
    var btn = document.getElementById(BTN_ID);
    frame.style.display = open ? 'block' : 'none';
    if (btn) btn.textContent = open ? '×' : '💬';
    if (open) {
      frame.contentWindow && frame.contentWindow.postMessage({ type: 'goadmin-im-open' }, '*');
    }
  }

  function boot() {
    ensureStyles();
    if (document.getElementById(BTN_ID)) return;
    var btn = document.createElement('button');
    btn.id = BTN_ID;
    btn.type = 'button';
    btn.setAttribute('aria-label', '打开在线客服');
    btn.textContent = '💬';
    btn.addEventListener('click', function () {
      setOpen(!open);
    });
    document.body.appendChild(btn);

    window.addEventListener('message', function (ev) {
      var data = ev.data || {};
      if (data.type === 'goadmin-im-close') setOpen(false);
      if (data.type === 'goadmin-im-ready') {
        /* iframe ready */
      }
    });

    window.GoAdminIM = {
      open: function () { setOpen(true); },
      close: function () { setOpen(false); },
      toggle: function () { setOpen(!open); },
      setContext: function (ctx) {
        var frame = document.getElementById(FRAME_ID);
        if (frame && frame.contentWindow) {
          frame.contentWindow.postMessage({ type: 'goadmin-im-context', context: ctx || {} }, '*');
        }
        window.__GOADMIN_IM_CONTEXT__ = ctx || {};
      },
    };
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', boot);
  } else {
    boot();
  }
})();
