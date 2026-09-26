// The On track score as the app shows it: a ring, a verdict, and the plain
// sentence that says what the number is made of.

export default function OnTrackDemo() {
  const size = 200, stroke = 16, r = (size - stroke) / 2, c = 2 * Math.PI * r, p = 92;
  return (
    <div className="otd" role="img" aria-label="On track: 92%. Of 48 actions trackline could check, 44 matched what you asked.">
      <span className="otd-ring">
        <svg viewBox={`0 0 ${size} ${size}`} aria-hidden="true">
          <circle cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={stroke} stroke="#e7e7e3" />
          <circle cx={size / 2} cy={size / 2} r={r} fill="none" strokeWidth={stroke} stroke="#1f9d52" strokeLinecap="round"
            strokeDasharray={c} strokeDashoffset={c * (1 - p / 100)} transform={`rotate(-90 ${size / 2} ${size / 2})`}
            className="otd-value" style={{ ["--c" as string]: c }} />
        </svg>
        <b>{p}%</b>
      </span>
      <div className="otd-text">
        <span className="otd-verdict">Mostly on track</span>
        <p>Of <b>48</b> actions trackline could check this week, <b>44</b> matched what you asked, and <b>4</b> did not.</p>
        <ul>
          <li><span>Off-limits files</span><i>2 · stopped</i></li>
          <li><span>New dependencies</span><i>1</i></li>
          <li><span>Outside the request</span><i>1</i></li>
        </ul>
      </div>
    </div>
  );
}
