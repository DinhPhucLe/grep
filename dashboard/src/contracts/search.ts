export type PersonRecommendation = {
  id: string;
  name: string;
  mail: string;
};

export type OrgRecommendation = {
  id: string;
  name: string;
};

export type RecommendationsResponse<T> = {
  items: T[];
};
