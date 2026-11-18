# Parcel UI

- Keep the application header to the brand and account controls.
- Keep map status compact. Show a county/filter summary only when filters are active. Keep search and panel positions fixed across zoom levels; omit visible view-level headings.
- Select counties through map outlines or keyboard-accessible county labels. Use coverage-page links when the map is unavailable. Keep the county dropdown off the map.
- Keep ingestion details and the full coverage list on `/coverage`. Reserve the map's right-hand panel for parcel results and details. Keep county selection, locked labels, and a short locked-county notice on the map.
- Preserve discovery controls when changing map data. The approved regional view hides parcel controls without clearing their state. Explicit county selection sets the search area; zoom changes display detail only. Show parcel controls in the selected configured county at county and parcel zoom levels. Show a locked explanation for other counties. Mark parcel controls unavailable until real data is connected. The grouped filter layout is approved. Omit mock and preview badges. Until real data is connected, keep a short data-availability notice; selections must not change parcel results, map evidence, or make data requests.

- Use blue focus outlines for controls. Keep keyboard focus visible; do not use the selected parcel's amber color for control focus.

- Make active-filter labels removal buttons. They must clear only their own filter, update the map and list, and preserve the filter panel's open state. Clear the corresponding numeric field when removing an acreage bound. Keep keyboard focus on another active filter or the filter arrow after removal.

- Keep all visible parcel boundaries on the map as neutral context. Require at least one active filter before showing match results or blue match highlights. With no active filters, show zero matches. Search and map selection may inspect a nonmatching parcel without adding it to the filter results. Selecting a finder suggestion opens details directly; do not add a separate Go button.

- Use Skeleton for standard controls, form styles, and shared component behavior. Check its current API before creating a custom standard widget. Keep custom code for parcel/map behavior and the approved sliding-panel layout. Set approved colors through Skeleton theme properties in `src/theme.css`. Keep Google's pre-approved button artwork unchanged.

- Keep the map as the main discovery view on desktop and tablet. Let it fill the available application area. Use compact search and filter controls over the map. Keep filters collapsible and parcel details dismissible. Verify the default map area and panel behavior at both screen sizes after layout changes.
- Use only the left-side filter panel. The layout is approved. Require operator approval before adding layout alternatives. Put open/close arrow controls on the left and right panel edges, not beside search. Slide panels toward their screen edge when hidden. Honor reduced-motion preferences. Use only edge arrows to hide filters and results; keep the parcel-details × action to return to the list. Keep hidden controls outside keyboard navigation.
- Use one right-hand panel. Show all matching results when no parcel is selected, unless the user hides the list. Provide Hide matches and Show matches controls. Hiding the list preserves filters, map highlights, list mode, and page. Replace the list with parcel details on selection. Restore the list when the user closes details. Preserve filters, list mode, and page during this change.
- Keep filters open until the user closes them. Use the same matching parcel IDs for the list and map highlights, including while details are open. Keep the selected parcel outline distinct.
- Use dark mode only: a softer graphite map background (#24272b), lighter charcoal panels (#303438), light gray text, blue match highlights, and an amber selected-parcel outline. Apply the palette to the map, native form controls, and map controls. Test with both light and dark operating-system preferences.
