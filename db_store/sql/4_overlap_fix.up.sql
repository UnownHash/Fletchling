-- Fix procedure for overlap disablement
-- I guess ST_Overlaps() is false if fully contained?
-- So, just check we have Poly/MPoly before doing ST_Area().
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
        ST_GeometryType(ST_Intersection(a.polygon, b.polygon)) IN ('Polygon', 'MultiPolygon') AND
        (100 * ST_Area(ST_Intersection(a.polygon,b.polygon)) / ST_Area(b.polygon)) > maximum_overlap
  );
  UPDATE nests a, overlapNest b SET a.active=0, discarded = 'overlap' WHERE a.nest_id=b.nest_id;
  DROP TEMPORARY TABLE overlapNest;
END
