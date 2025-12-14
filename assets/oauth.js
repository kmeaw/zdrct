'use strict';

window.addEventListener('DOMContentLoaded', (event) => {
  const $message = document.getElementById('message');

  let usp = new URLSearchParams(location.hash.substr(1));
  if (usp.size == 0) {
    usp = new URLSearchParams(location.search.substr(1));
  }
  const entries = Object.fromEntries(usp.entries());
  let target = '/oauth';
  let tab = 'twitch';
  if (usp.get('scope') == 'https://www.googleapis.com/auth/youtube') {
    target = '/oauth/youtube';
    tab = 'youtube';
  }
  if (entries.error) {
    $message.innerText = entries.error;
    if (entries.error_description) {
      $message.innerText = entries.error + ': ' + entries.error_description;
    }
    return;
  }
  fetch(target, {
    method: 'POST',
    mode: 'same-origin',
    cache: 'no-cache',
    credentials: 'omit',
    headers: {
      'Content-Type': 'application/json'
    },
    redirect: 'error',
    body: JSON.stringify(entries)
  })
    .then((r) => {
      if (!r.ok) {
	return r.text().then((t) => { throw t; });
      }

      return r.json();
    })
    .then((result) => {
      if (!result.ok) {
	throw 'Bad result: ' + JSON.stringify(result);
      }

      location.replace('/?tab=' + encodeURIComponent(tab));
    })
    .catch((e) => $message.innerText = 'An error has occurred: ' + e);
});
