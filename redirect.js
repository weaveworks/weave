var target = "http://docs.weave.works/weave/latest_release/";

try {
  var pathname = window.location.pathname.split("/");
  window.location.replace(target + pathname[pathname.length-1] + window.location.hash)
} catch (e) {
  window.location.replace(target)
}
