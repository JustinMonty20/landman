// Candidates retain the chosen offset for ambiguous daylight-saving hours. Empty means a skipped hour.
export function dueCandidates(local: string, zone: string): { label: string; due_at: string }[] {
 if (!/^\d{4}-\d\d-\d\dT\d\d:\d\d$/.test(local)) return [];
 const naive = Date.parse(local + ':00Z');
 if (!Number.isFinite(naive)) return [];
 let formatter: Intl.DateTimeFormat;
 try { formatter = new Intl.DateTimeFormat('en-CA', {timeZone:zone,year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hourCycle:'h23'}); }
 catch { return []; }
 const found: {label:string;due_at:string}[]=[];
 const seen = new Set<string>();
 for (let offset=-14*60;offset<=14*60;offset+=15) {
  const instant=new Date(naive-offset*60_000);
  const parts=Object.fromEntries(formatter.formatToParts(instant).map(p=>[p.type,p.value]));
  if (`${parts.year}-${parts.month}-${parts.day}T${parts.hour}:${parts.minute}`!==local) continue;
  const sign=offset<0?'-':'+'; const abs=Math.abs(offset);
  const suffix=`${sign}${String(Math.floor(abs/60)).padStart(2,'0')}:${String(abs%60).padStart(2,'0')}`;
  if (seen.has(suffix)) continue;
  seen.add(suffix);
  found.push({label:`UTC${suffix}`,due_at:`${local}:00${suffix}`});
 }
 return found;
}
export function smsCapacity(message:string): {remaining:number; encoding:string} {
 const basic="@£$¥èéùìòÇ\nØø\rÅåΔ_ΦΓΛΩΠΨΣΘΞ ÆæßÉ !\"#¤%&'()*+,-./0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿abcdefghijklmnopqrstuvwxyzäöñüà";
 const extended='^{}\\[~]|€\f';
 let units=0; let gsm=true;
 for(const char of message){ if(basic.includes(char))units++; else if(extended.includes(char))units+=2; else gsm=false; }
 if(gsm)return {remaining:160-units,encoding:'GSM-7'};
 return {remaining:70-[...message].reduce((sum,char)=>sum+(char.codePointAt(0)!>0xffff?2:1),0),encoding:'Unicode'};
}
