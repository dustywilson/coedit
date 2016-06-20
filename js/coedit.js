function newCoeditSource(instanceName) {
  return new EventSource("/coedit/"+instanceName+"?s="+Math.floor(Math.random()*100000))
}
