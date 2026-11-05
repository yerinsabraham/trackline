// The logo's geometry, drawn once: the mark in the nav, the share cards and the
// launch film all trace these numbers, so the shape cannot drift between them.
// Units are those of the 1254px master the logo was drawn at.
export const LOGO = {
  curve: "M195 866 C330 866 480 425 627 425 C774 425 924 866 1060 866",
  stroke: 112,
  ball: { cx: 627, cy: 393, r: 118 },
  orange: "#fd6a06",
  // The shape alone, cropped square around it, for use beside a word.
  bare: "120 91 1014 1014",
  // The shape on its black tile, as the app icon shows it.
  tile: "0 0 1254 1254",
} as const;
