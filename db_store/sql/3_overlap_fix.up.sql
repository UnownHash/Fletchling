-- Fix procedure for overlap disablement
-- mysql (at least old 8.0.x)'s ST_Area throws an
-- error on non-poly/mpoly (2D objects). (MariaDB's
-- ST_Area will return 0 for non-poly/mpoly.)
-- It looks like ST_Overlaps would work as a replacement
-- for ST_Intersects which gives us what we want. However,
-- that is very slow. Keeping the ST_Intersects in front
-- of ST_Overlaps keeps it fast.
DROP PROCEDURE IF EXISTS fl_nest_filter_overlap;
CREATE PROCEDURE fl_nest_filter_overlap (IN maximum_overlap double)
BEGIN
  DROP TEMPORARY TABLE IF EXISTS overlapNest;
  CREATE TEMPORARY TABLE overlapNest AS (
    SELECT b.nest_id
    FROM nests a, nests b
    WHERE a.active = 1 AND b.active = 1 AND
        a.m2 > b.m2 AND
        ST_Intersects(a.polygon, b.polygon) AND
        ST_Overlaps(a.polygon, b.polygon) AND
        (100 * ST_Area(ST_Intersection(a.polygon,b.polygon)) / ST_Area(b.polygon)) > maximum_overlap
  );
  UPDATE nests a, overlapNest b SET a.active=0, discarded = 'overlap' WHERE a.nest_id=b.nest_id;
  DROP TEMPORARY TABLE overlapNest;
END
