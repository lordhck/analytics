(function () {
  var script = document.currentScript;
  var site = script.getAttribute("data-site");
  var endpoint = new URL(script.src).origin + "/api/event";

  function send() {
    var body = JSON.stringify({
      site: site,
      path: location.pathname,
      referrer: document.referrer
    });
    if (navigator.sendBeacon) {
      navigator.sendBeacon(endpoint, body);
    } else {
      fetch(endpoint, { method: "POST", body: body, keepalive: true });
    }
  }

  var push = history.pushState;
  history.pushState = function () {
    push.apply(this, arguments);
    send();
  };
  window.addEventListener("popstate", send);

  send();
})();
