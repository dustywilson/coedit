function newCoeditSource(instanceName, name) {
  return new EventSource("/coedit/"+instanceName+"?n="+name+"&s="+Math.floor(Math.random()*100000))
}
